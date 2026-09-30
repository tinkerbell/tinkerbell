package kube

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
)

// fakeInformers hands out one fake informer per object type.
type fakeInformers struct {
	mu   sync.Mutex
	byID map[string]*safeInformer
}

// safeInformer guards controllertest.FakeInformer, whose handler list is not safe for concurrent use.
type safeInformer struct {
	mu sync.Mutex
	*controllertest.FakeInformer
}

func (s *safeInformer) AddEventHandler(h toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.FakeInformer.AddEventHandler(h)
}

func (s *safeInformer) Add(obj metav1.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FakeInformer.Add(obj)
}

func (s *safeInformer) Update(oldObj, newObj metav1.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FakeInformer.Update(oldObj, newObj)
}

func (f *fakeInformers) GetInformer(_ context.Context, obj client.Object, _ ...cache.InformerGetOption) (cache.Informer, error) {
	return f.informer(obj), nil
}

func (f *fakeInformers) informer(obj client.Object) *safeInformer {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprintf("%T/%v", obj, obj.GetObjectKind().GroupVersionKind())
	if f.byID == nil {
		f.byID = map[string]*safeInformer{}
	}
	if f.byID[id] == nil {
		f.byID[id] = &safeInformer{FakeInformer: controllertest.NewFakeInformer(controllertest.Synced)}
	}
	return f.byID[id]
}

// fakeHardware is the stored Hardware the store reads.
type fakeHardware struct {
	mu  sync.Mutex
	hws map[types.NamespacedName]*tinkerbell.Hardware
}

func (f *fakeHardware) set(hw *tinkerbell.Hardware) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hws == nil {
		f.hws = map[types.NamespacedName]*tinkerbell.Hardware{}
	}
	f.hws[client.ObjectKeyFromObject(hw)] = hw
}

func (f *fakeHardware) get(_ context.Context, key types.NamespacedName) (*tinkerbell.Hardware, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if hw, ok := f.hws[key]; ok {
		return hw.DeepCopy(), nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "hardware"}, key.Name)
}

type fakeResolver struct {
	mu    sync.Mutex
	refs  map[string]any
	err   error
	calls int
}

func (f *fakeResolver) resolve(_ context.Context, _ *tinkerbell.Hardware) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.refs, f.err
}

func (f *fakeResolver) set(refs map[string]any, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refs, f.err = refs, err
}

func newTestStore(hws *fakeHardware, res *fakeResolver, inf *fakeInformers) *renderStore {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	return newRenderStore(logr.Discard(), inf, mapper, hws.get, res.resolve)
}

func templated(rv string) *tinkerbell.Hardware {
	return &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "tink", ResourceVersion: rv},
		Spec: tinkerbell.HardwareSpec{
			References: map[string]tinkerbell.Reference{"net": {Version: "v1", Resource: "configmaps", Namespace: "tink", Name: "net1"}},
			UserData:   ptr("domain={{ .references.net.data.domain }}"),
		},
	}
}

func netRefs(domain string) map[string]any {
	return map[string]any{"net": map[string]any{"data": map[string]any{"domain": domain}}}
}

func TestRenderStoreServesRenderedHardware(t *testing.T) {
	ctx := context.Background()
	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	s := newTestStore(hws, res, &fakeInformers{})
	hw := templated("1")
	hws.set(hw)

	if _, ok := s.rendered(hw); ok {
		t.Fatal("templated Hardware must not be served before it is rendered")
	}
	if !s.render(ctx, client.ObjectKeyFromObject(hw)) {
		t.Fatal("render failed")
	}
	if res.calls != 1 || len(s.entries) != 1 {
		t.Fatalf("resolved references %d times and stored %d entries, want 1 each", res.calls, len(s.entries))
	}
	got, ok := s.rendered(hw)
	if !ok || *got.Spec.UserData != "domain=example.org" {
		t.Fatalf("rendered = %v, %v", got, ok)
	}
	got.Spec.UserData = ptr("mutated")
	if again, _ := s.rendered(hw); *again.Spec.UserData != "domain=example.org" {
		t.Fatal("a caller's copy must not change the stored rendering")
	}
}

func TestRenderStoreUntemplatedHardware(t *testing.T) {
	ctx := context.Background()
	hws, res := &fakeHardware{}, &fakeResolver{err: errors.New("reference denied")}
	s := newTestStore(hws, res, &fakeInformers{})
	hw := templated("1")
	hw.Spec.UserData = ptr("#cloud-config")
	hws.set(hw)

	if got, ok := s.rendered(hw); !ok || got != hw {
		t.Fatal("untemplated Hardware must be served as stored before any render")
	}
	if !s.render(ctx, client.ObjectKeyFromObject(hw)) {
		t.Fatal("render failed")
	}
	if got, ok := s.rendered(hw); !ok || got != hw {
		t.Fatal("untemplated Hardware must be served as stored")
	}
	if res.calls != 0 {
		t.Fatalf("references resolved %d times for untemplated Hardware, want 0", res.calls)
	}
}

func TestRenderStoreSkippedJinja(t *testing.T) {
	hws, res := &fakeHardware{}, &fakeResolver{err: errors.New("reference denied")}
	s := newTestStore(hws, res, &fakeInformers{})
	hw := templated("1")
	hw.Annotations = map[string]string{templateSkipAnnotation: `["spec.userData"]`}
	hw.Spec.UserData = ptr("{{ ds.meta_data.hostname }}")
	hws.set(hw)
	if got, ok := s.rendered(hw); !ok || got != hw {
		t.Fatal("skipped Jinja must be served unchanged before rendering")
	}
	if !s.render(context.Background(), client.ObjectKeyFromObject(hw)) {
		t.Fatal("render of skipped Jinja failed")
	}
	if got, ok := s.rendered(hw); !ok || got != hw {
		t.Fatal("skipped Jinja must be served unchanged after rendering")
	}
	if res.calls != 0 || len(s.refs) != 0 {
		t.Fatal("skipped Jinja must not resolve or track references")
	}
}

func TestRenderStoreInvalidSkipAnnotation(t *testing.T) {
	hws, res := &fakeHardware{}, &fakeResolver{}
	s := newTestStore(hws, res, &fakeInformers{})
	hw := templated("1")
	hw.Spec.UserData = ptr("#cloud-config")
	hw.Annotations = map[string]string{templateSkipAnnotation: "null"}
	hws.set(hw)
	if _, ok := s.rendered(hw); ok {
		t.Fatal("invalid skip annotation must not take the unchanged-object fast path")
	}
	if s.render(context.Background(), client.ObjectKeyFromObject(hw)) {
		t.Fatal("invalid skip annotation must fail rendering")
	}
	if _, ok := s.rendered(hw); ok {
		t.Fatal("failed Hardware without a previous rendering must not be served")
	}
}

func TestRenderStoreUnusedDeniedReference(t *testing.T) {
	hws, res := &fakeHardware{}, &fakeResolver{refs: map[string]any{}, err: errors.New("reference denied")}
	s := newTestStore(hws, res, &fakeInformers{})
	hw := templated("1")
	hw.Spec.UserData = ptr("host={{ .hardware.metadata.name }}")
	hws.set(hw)

	if !s.render(context.Background(), client.ObjectKeyFromObject(hw)) {
		t.Fatal("a denied reference no template uses must not fail the render")
	}
	if got, ok := s.rendered(hw); !ok || *got.Spec.UserData != "host=m1" {
		t.Fatalf("rendered = %v, %v", got, ok)
	}
}

func TestRenderStoreFailureServesPrevious(t *testing.T) {
	ctx := context.Background()
	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	s := newTestStore(hws, res, &fakeInformers{})
	key := types.NamespacedName{Namespace: "tink", Name: "m1"}
	hws.set(templated("1"))
	s.render(ctx, key)

	broken := templated("2")
	broken.Spec.UserData = ptr("{{ .references.missing.x }}")
	hws.set(broken)
	if s.render(ctx, key) {
		t.Fatal("render of a broken template must fail")
	}
	if got, ok := s.rendered(broken); !ok || *got.Spec.UserData != "domain=example.org" {
		t.Fatalf("rendered = %v, %v; want the previous rendering", got, ok)
	}

	hws.set(templated("3"))
	res.set(netRefs("fixed.org"), nil)
	s.render(ctx, key)
	if got, _ := s.rendered(templated("3")); *got.Spec.UserData != "domain=fixed.org" {
		t.Fatalf("userData = %q after the fix", *got.Spec.UserData)
	}
}

func TestRenderStoreNeverRendered(t *testing.T) {
	hws, res := &fakeHardware{}, &fakeResolver{}
	s := newTestStore(hws, res, &fakeInformers{})
	hw := templated("1")
	hw.Spec.UserData = ptr("{{ .references.missing.x }}")
	hws.set(hw)

	if s.render(context.Background(), client.ObjectKeyFromObject(hw)) {
		t.Fatal("render must fail")
	}
	if _, ok := s.rendered(hw); ok {
		t.Fatal("Hardware that never rendered must not be served")
	}
}

func TestRenderStoreReferenceChangeRequeues(t *testing.T) {
	ctx := context.Background()
	hws, res, inf := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}, &fakeInformers{}
	s := newTestStore(hws, res, inf)
	hw := templated("1")
	hws.set(hw)
	s.render(ctx, client.ObjectKeyFromObject(hw))

	cm := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Namespace: "tink", Name: "net1"}}
	cm.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
	other := cm.DeepCopy()
	other.Name = "unrelated"
	cmInformer := inf.informer(cm)

	cmInformer.Update(other, other)
	if s.queue.Len() != 0 {
		t.Fatal("an unreferenced object must not requeue anything")
	}
	cmInformer.Update(cm, cm)
	if s.queue.Len() != 1 {
		t.Fatalf("queue length = %d, want 1", s.queue.Len())
	}
	if key, _ := s.queue.Get(); key != client.ObjectKeyFromObject(hw) {
		t.Fatalf("queued %v", key)
	}
}

func TestRenderStoreForgetsDeletedHardware(t *testing.T) {
	ctx := context.Background()
	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	s := newTestStore(hws, res, &fakeInformers{})
	hw := templated("1")
	hws.set(hw)
	s.render(ctx, client.ObjectKeyFromObject(hw))

	hws.hws = nil
	if !s.render(ctx, client.ObjectKeyFromObject(hw)) {
		t.Fatal("rendering a deleted Hardware must succeed")
	}
	if len(s.entries) != 0 || len(s.refs) != 0 || len(s.referrers) != 0 {
		t.Fatalf("state left behind: %d entries, %d refs, %d referrers", len(s.entries), len(s.refs), len(s.referrers))
	}
}

func TestRenderStoreStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	hws, res, inf := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}, &fakeInformers{}
	s := newTestStore(hws, res, inf)
	done := make(chan error)
	go func() { done <- s.Start(ctx, 2) }()

	hw := templated("1")
	hws.set(hw)
	// Re-send the event until Start has registered its handler and a worker rendered it.
	for ; ; time.Sleep(time.Millisecond) {
		inf.informer(&tinkerbell.Hardware{}).Add(hw)
		if got, ok := s.rendered(hw); ok {
			if *got.Spec.UserData != "domain=example.org" {
				t.Fatalf("userData = %q", *got.Spec.UserData)
			}
			break
		}
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
