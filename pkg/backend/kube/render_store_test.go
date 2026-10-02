package kube

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	metadatafake "k8s.io/client-go/metadata/fake"
	clienttesting "k8s.io/client-go/testing"
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
	failRegistrations int
	registrations     int
}

func (s *safeInformer) AddEventHandler(h toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failRegistrations > 0 {
		s.failRegistrations--
		return nil, errors.New("temporary handler registration failure")
	}
	s.registrations++
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

type fakeMetadataFactory struct {
	fakeInformers
}

func (f *fakeMetadataFactory) ForResource(gvr schema.GroupVersionResource) informers.GenericInformer {
	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: gvr.Group, Version: gvr.Version, Kind: gvr.Resource})
	return &fakeMetadataInformer{informer: f.informer(obj), resource: gvr.GroupResource()}
}

func (*fakeMetadataFactory) Start(<-chan struct{}) {}
func (*fakeMetadataFactory) Shutdown()             {}

type fakeMetadataInformer struct {
	informer *safeInformer
	resource schema.GroupResource
}

func (f *fakeMetadataInformer) Informer() toolscache.SharedIndexInformer { return f.informer }
func (f *fakeMetadataInformer) Lister() toolscache.GenericLister {
	return toolscache.NewGenericLister(f.informer.GetIndexer(), f.resource)
}

func newTestStore(hws *fakeHardware, res *fakeResolver, inf *fakeInformers) *renderStore {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	store := newRenderStore(logr.Discard(), inf, mapper, nil, hws.get, res.resolve, prometheus.NewRegistry())
	store.referenceInformers = &fakeMetadataFactory{}
	return store
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

func TestRenderStoreRecreatedHardware(t *testing.T) {
	for _, test := range []struct {
		name            string
		resourceVersion string
		identity        bool
	}{
		{name: "rendered same version", resourceVersion: "1"},
		{name: "rendered new version", resourceVersion: "2"},
		{name: "identity same version", resourceVersion: "1", identity: true},
		{name: "identity new version", resourceVersion: "2", identity: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("old.org")}
			s := newTestStore(hws, res, &fakeInformers{})
			original := templated("1")
			original.UID = "old-uid"
			if test.identity {
				original.Spec.UserData = ptr("#cloud-config")
			}
			hws.set(original)
			key := client.ObjectKeyFromObject(original)
			if !s.render(ctx, key) {
				t.Fatal("initial render failed")
			}
			replacement := templated(test.resourceVersion)
			replacement.UID = "new-uid"
			replacement.Spec.UserData = ptr("{{ .references.missing.x }}")
			hws.set(replacement)
			if _, ok := s.rendered(replacement); ok {
				t.Fatal("replacement must not inherit the previous UID's rendering before its first render")
			}
			if s.render(ctx, key) {
				t.Fatal("replacement render must fail")
			}
			if _, ok := s.rendered(replacement); ok || s.entries[key].good != nil {
				t.Fatal("replacement must not inherit the previous UID's rendering after failure")
			}
		})
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
	cmInformer := s.referenceInformers.ForResource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Informer().(*safeInformer)

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

func TestRenderStoreCrossNamespaceReference(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	metadataClient := metadatafake.NewSimpleMetadataClient(runtime.NewScheme())
	metadataClient.PrependReactor("list", "configmaps", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, &metav1.List{}, nil
	})
	updates := watch.NewRaceFreeFake()
	metadataClient.PrependWatchReactor("configmaps", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, updates, nil
	})
	hws, res, hardwareInformers := &fakeHardware{}, &fakeResolver{refs: netRefs("old.org")}, &fakeInformers{}
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	s := newRenderStore(logr.Discard(), hardwareInformers, mapper, metadataClient, hws.get, res.resolve, prometheus.NewRegistry())
	t.Cleanup(func() {
		cancel()
		s.queue.ShutDown()
		s.referenceInformers.Shutdown()
	})
	hw := templated("1")
	reference := hw.Spec.References["net"]
	reference.Namespace = "shared"
	hw.Spec.References["net"] = reference
	hws.set(hw)
	if !s.render(ctx, client.ObjectKeyFromObject(hw)) {
		t.Fatal("initial render failed")
	}
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	inf := s.referenceInformers.ForResource(gvr).Informer()
	if !toolscache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		t.Fatal("metadata informer did not sync")
	}
	updates.Add(&metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Namespace: "shared", Name: "net1", ResourceVersion: "2"}})
	if err := wait.PollUntilContextCancel(ctx, time.Millisecond, true, func(context.Context) (bool, error) {
		return s.queue.Len() == 1, nil
	}); err != nil {
		t.Fatalf("cross-namespace reference change did not requeue Hardware: %v", err)
	}
	res.set(netRefs("new.org"), nil)
	if !s.processNext(ctx) {
		t.Fatal("worker stopped before processing reference change")
	}
	if got, ok := s.rendered(hw); !ok || *got.Spec.UserData != "domain=new.org" {
		t.Fatalf("rendered = %v, %v; want updated cross-namespace data", got, ok)
	}
	if len(hardwareInformers.byID) != 0 {
		t.Fatal("reference informer must not come from the Hardware backend cache")
	}
	for _, action := range metadataClient.Actions() {
		if action.GetNamespace() != metav1.NamespaceAll {
			t.Fatalf("metadata watcher was unexpectedly namespace-scoped: %q", action.GetNamespace())
		}
	}
	cancel()
	s.referenceInformers.Shutdown()
	if !updates.IsStopped() || !inf.IsStopped() {
		t.Fatal("metadata informer must stop when its context is canceled")
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

type retryMapper struct {
	meta.RESTMapper
	mu       sync.Mutex
	failures int
	calls    int
}

func (mapper *retryMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	mapper.mu.Lock()
	defer mapper.mu.Unlock()
	mapper.calls++
	if mapper.failures > 0 {
		mapper.failures--
		return schema.GroupVersionKind{}, errors.New("temporary discovery failure")
	}
	return mapper.RESTMapper.KindFor(resource)
}

func TestRenderStoreHandlerRegistrationRetry(t *testing.T) {
	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	s := newTestStore(hws, res, &fakeInformers{})
	defer s.queue.ShutDown()
	inf := s.referenceInformers.ForResource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Informer().(*safeInformer)
	inf.failRegistrations = 1
	hw := templated("1")
	hw.Spec.UserData = ptr("host={{ .hardware.metadata.name }}")
	res.err = errors.New("unused reference denied")
	hws.set(hw)
	key := client.ObjectKeyFromObject(hw)
	s.queue.Add(key)
	s.processNext(context.Background())
	if s.queue.NumRequeues(key) != 1 || len(s.watched) != 0 {
		t.Fatal("handler registration failure must retry even with an unused denied reference")
	}
	s.queue.Add(key)
	s.processNext(context.Background())
	if inf.registrations != 1 || len(s.watched) != 1 || s.queue.NumRequeues(key) != 0 {
		t.Fatal("retry must install one handler and clear registration backoff")
	}
}

func TestRenderStoreWatchRegistrationRetry(t *testing.T) {
	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	s := newTestStore(hws, res, &fakeInformers{})
	defer s.queue.ShutDown()
	mapper := &retryMapper{RESTMapper: s.mapper, failures: 1}
	s.mapper = mapper
	hw := templated("1")
	hws.set(hw)
	key := client.ObjectKeyFromObject(hw)
	s.queue.Add(key)
	s.processNext(context.Background())
	if got, ok := s.rendered(hw); !ok || *got.Spec.UserData != "domain=example.org" {
		t.Fatal("successful rendering must remain available while watch setup retries")
	}
	if s.queue.NumRequeues(key) != 1 || len(s.watched) != 0 {
		t.Fatal("failed registration must schedule a retry without claiming the resource is watched")
	}
	s.queue.Add(key)
	s.processNext(context.Background())
	if mapper.calls != 2 || len(s.watched) != 1 || s.queue.NumRequeues(key) != 0 {
		t.Fatal("registration retry must recover and clear backoff")
	}
}

func TestRenderStoreConcurrentWatchRegistration(t *testing.T) {
	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	s := newTestStore(hws, res, &fakeInformers{})
	defer s.queue.ShutDown()
	mapper := &retryMapper{RESTMapper: s.mapper, failures: 1}
	s.mapper = mapper
	var workers sync.WaitGroup
	for _, name := range []string{"m1", "m2"} {
		hw := templated("1")
		hw.Name = name
		hws.set(hw)
		workers.Go(func() { s.render(context.Background(), client.ObjectKeyFromObject(hw)) })
	}
	workers.Wait()
	if mapper.calls != 2 || len(s.watched) != 1 {
		t.Fatal("concurrent registration must retry a failed attempt and install exactly one watcher")
	}
	if !s.render(context.Background(), types.NamespacedName{Namespace: "tink", Name: "m1"}) || mapper.calls != 2 {
		t.Fatal("an established watcher must not be registered again")
	}
	inf := s.referenceInformers.ForResource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Informer().(*safeInformer)
	if inf.registrations != 1 {
		t.Fatal("concurrent workers must install exactly one handler")
	}
}

func TestRenderStoreMetricsAndNotifications(t *testing.T) {
	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	s := newTestStore(hws, res, &fakeInformers{})
	defer s.queue.ShutDown()
	ctx := context.Background()
	hw := templated("1")
	key := client.ObjectKeyFromObject(hw)
	hws.set(hw)
	if !s.render(ctx, key) {
		t.Fatal("initial render failed")
	}
	if len(s.notify) != 1 || len(s.TakeChanges()) != 1 {
		t.Fatal("successful rendering must notify consumers")
	}
	<-s.Changes()
	broken := hw.DeepCopy()
	broken.ResourceVersion = "2"
	broken.Spec.UserData = ptr("{{ .references.missing }}")
	hws.set(broken)
	if s.render(ctx, key) || testutil.ToFloat64(s.fallbacks) != 1 {
		t.Fatal("failed rendering must count the last-good fallback")
	}
	if !s.needsNotification(key) {
		t.Fatal("failure must notify consumers")
	}
	hws.set(hw)
	if !s.render(ctx, key) || testutil.ToFloat64(s.fallbacks) != 0 {
		t.Fatal("recovery must clear the fallback gauge")
	}
	s.forget(key)
	select {
	case <-s.Changes():
	default:
		t.Fatal("published results and deletion must wake the consumer")
	}
	keys := s.TakeChanges()
	if len(keys) != 1 || keys[0] != key || len(s.TakeChanges()) != 0 {
		t.Fatalf("changes must coalesce to the latest key: %v", keys)
	}
	if testutil.ToFloat64(s.renders.WithLabelValues("success")) != 2 || testutil.ToFloat64(s.renders.WithLabelValues("error")) != 1 {
		t.Fatal("render counters must distinguish successful and failed attempts")
	}
}

func (s *renderStore) needsNotification(key types.NamespacedName) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, pending := s.changes[key]
	return pending
}

func TestRenderStoreNotificationsSlowConsumer(t *testing.T) {
	s := newTestStore(&fakeHardware{}, &fakeResolver{}, &fakeInformers{})
	defer s.queue.ShutDown()
	var publishers sync.WaitGroup
	for index := range 100 {
		key := types.NamespacedName{Namespace: "tink", Name: fmt.Sprintf("machine-%d", index)}
		publishers.Go(func() {
			for range 3 {
				s.forget(key)
			}
		})
	}
	publishers.Wait()
	if len(s.notify) != 1 {
		t.Fatal("slow consumers must need at most one pending wakeup")
	}
	<-s.Changes()
	if len(s.TakeChanges()) != 100 {
		t.Fatal("coalescing must not drop changed Hardware keys")
	}
}

func TestRenderStoreHardwareUpdateFilter(t *testing.T) {
	const hardwareKind = "Hardware"
	before := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tinkerbell.org/v1alpha1", "kind": hardwareKind,
		"metadata": map[string]any{"name": "m1", "namespace": "tink", "uid": "uid-1", "resourceVersion": "1"},
		"spec":     map[string]any{"userData": "literal"},
	}}
	for _, test := range []struct {
		name  string
		path  []string
		value any
		want  bool
	}{
		{name: "resource version", path: []string{"metadata", "resourceVersion"}, value: "2"},
		{name: "managed fields", path: []string{"metadata", "managedFields"}, value: []any{map[string]any{"manager": "hardware-controller"}}},
		{name: "renderer condition", path: []string{"status", "conditions"}, value: []any{map[string]any{"type": "Rendered", "status": "False"}}},
		{name: "other condition", path: []string{"status", "conditions"}, value: []any{map[string]any{"type": "Ready", "status": "True"}}, want: true},
		{name: "mixed conditions", path: []string{"status", "conditions"}, value: []any{map[string]any{"type": "Rendered"}, map[string]any{"type": "Ready"}}, want: true},
		{name: "condition and attributes", path: []string{"status"}, value: map[string]any{"conditions": []any{map[string]any{"type": "Rendered"}}, "attributes": map[string]any{"inBand": map[string]any{"collectionMethod": "agent"}}}, want: true},
		{name: "skip annotation", path: []string{"metadata", "annotations"}, value: map[string]any{templateSkipAnnotation: `["spec.userData"]`}, want: true},
		{name: "label", path: []string{"metadata", "labels"}, value: map[string]any{"rack": "new"}, want: true},
		{name: "attribute", path: []string{"status", "attributes", "inBand", "collectionMethod"}, value: "agent", want: true},
		{name: "spec", path: []string{"spec", "userData"}, value: "changed", want: true},
		{name: "UID", path: []string{"metadata", "uid"}, value: "uid-2", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			after := before.DeepCopy()
			if err := unstructured.SetNestedField(after.Object, test.value, test.path...); err != nil {
				t.Fatal(err)
			}
			s := newTestStore(&fakeHardware{}, &fakeResolver{}, &fakeInformers{})
			defer s.queue.ShutDown()
			original, originalAfter := before.DeepCopy(), after.DeepCopy()
			s.hardwareHandler().OnUpdate(before, after)
			if got := s.queue.Len() != 0; got != test.want {
				t.Fatalf("queued = %v, want %v", got, test.want)
			}
			if !reflect.DeepEqual(before.Object, original.Object) || !reflect.DeepEqual(after.Object, originalAfter.Object) {
				t.Fatal("filter modified an informer input")
			}
		})
	}
}

func BenchmarkRenderStoreLookup(b *testing.B) {
	for _, kind := range []string{"literal", "skipped-Jinja", "templated"} {
		for _, state := range []string{"steady", "startup", "version-mismatch", "startup-observed", "version-mismatch-observed"} {
			b.Run(kind+"/"+state, func(b *testing.B) {
				s := newTestStore(&fakeHardware{}, &fakeResolver{}, &fakeInformers{})
				defer s.queue.ShutDown()
				s.get = func(context.Context, types.NamespacedName) (*tinkerbell.Hardware, error) {
					panic("lookup must not read Hardware")
				}
				s.resolve = func(context.Context, *tinkerbell.Hardware) (map[string]any, error) {
					panic("lookup must not resolve references")
				}
				fleet := make([]*tinkerbell.Hardware, 10000)
				for index := range fleet {
					hw := templated("2")
					hw.Name = fmt.Sprintf("machine-%d", index)
					hw.UID = types.UID(hw.Name)
					switch kind {
					case "literal":
						hw.Spec.UserData = ptr("#cloud-config")
					case "skipped-Jinja":
						hw.Annotations = map[string]string{templateSkipAnnotation: `["spec.userData"]`}
						hw.Spec.UserData = ptr("{{ ds.meta_data.hostname }}")
					}
					fleet[index] = hw
					if state != "startup" && state != "startup-observed" {
						good := hw.DeepCopy()
						good.Spec.UserData = ptr("rendered")
						version := hw.ResourceVersion
						if strings.HasPrefix(state, "version-mismatch") {
							version = "1"
							good.ResourceVersion = version
						}
						s.entries[client.ObjectKeyFromObject(hw)] = &renderEntry{uid: hw.UID, resourceVersion: version, good: good, identity: kind != "templated"}
					}
					if strings.HasSuffix(state, "observed") {
						s.rememberDecision(hw)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					s.rendered(fleet[iteration%len(fleet)])
				}
				b.StopTimer()
			})
		}
	}
}

func TestRenderStoreDecisionIdentity(t *testing.T) {
	s := newTestStore(&fakeHardware{}, &fakeResolver{}, &fakeInformers{})
	defer s.queue.ShutDown()
	hw := templated("1")
	hw.UID = "original"
	hw.Spec.UserData = ptr("{{ ds.meta_data.hostname }}")
	hw.Annotations = map[string]string{templateSkipAnnotation: `["spec.userData"]`}
	s.hardwareHandler().OnAdd(hw, false)
	key := client.ObjectKeyFromObject(hw)
	if decision := s.decisions[key]; decision.needed || decision.uid != hw.UID || decision.resourceVersion != hw.ResourceVersion {
		t.Fatal("informer observation must prepare the skip-aware render decision")
	}
	replacement := hw.DeepCopy()
	delete(replacement.Annotations, templateSkipAnnotation)
	replacement.ResourceVersion = "2"
	if _, ok := s.rendered(replacement); ok {
		t.Fatal("a decision for an older version must not serve unrendered Jinja")
	}
	replacement.UID = "replacement"
	replacement.ResourceVersion = "1"
	if _, ok := s.rendered(replacement); ok {
		t.Fatal("a decision for an older UID must not serve unrendered Jinja")
	}
	invalid := hw.DeepCopy()
	invalid.ResourceVersion = "2"
	invalid.Annotations[templateSkipAnnotation] = "null"
	s.hardwareHandler().OnUpdate(hw, invalid)
	if _, ok := s.rendered(invalid); ok {
		t.Fatal("observed invalid configuration must not bypass rendering")
	}
	s.forget(key)
	if len(s.decisions) != 0 {
		t.Fatal("deleting Hardware must release its render decision")
	}
}

func TestRenderStoreStartupFailure(t *testing.T) {
	inf := &fakeInformers{}
	inf.informer(&tinkerbell.Hardware{}).failRegistrations = 1
	s := newTestStore(&fakeHardware{}, &fakeResolver{}, inf)
	if err := s.Start(context.Background(), 2); err == nil {
		t.Fatal("initial handler registration must fail")
	}
	if !s.queue.ShuttingDown() {
		t.Fatal("startup failure must shut down the queue")
	}
	if _, open := <-s.Changes(); open {
		t.Fatal("startup failure must close the notification channel")
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
	for range s.Changes() {
	}
}
