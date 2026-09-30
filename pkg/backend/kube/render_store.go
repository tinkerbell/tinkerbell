package kube

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/template/render"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// renderStore renders Hardware in the background, so that request paths such as
// DHCP and iPXE only look up results and never render or call the API server.
//
// Referenced objects are read live by the workers. Informers on their types
// hold metadata only and are used just to notice changes.
type renderStore struct {
	informers informerGetter
	mapper    meta.RESTMapper
	get       func(context.Context, types.NamespacedName) (*tinkerbell.Hardware, error)
	resolve   func(context.Context, *tinkerbell.Hardware) (map[string]any, error)
	queue     workqueue.TypedRateLimitingInterface[types.NamespacedName]
	log       logr.Logger

	mu        sync.RWMutex
	entries   map[types.NamespacedName]*renderEntry
	refs      map[types.NamespacedName][]refKey
	referrers map[refKey]map[types.NamespacedName]struct{}
	watched   map[schema.GroupResource]struct{}
}

type informerGetter interface {
	GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error)
}

// renderEntry is replaced whole, never modified, so readers need no lock on it.
type renderEntry struct {
	resourceVersion string               // of the Hardware last rendered
	err             error                // rendering that resourceVersion failed
	good            *tinkerbell.Hardware // last successful rendering, nil if none
	identity        bool                 // good is the stored object: nothing needed rendering
}

type refKey struct {
	group, resource, namespace, name string
}

func newRenderStore(
	log logr.Logger,
	informers informerGetter,
	mapper meta.RESTMapper,
	get func(context.Context, types.NamespacedName) (*tinkerbell.Hardware, error),
	resolve func(context.Context, *tinkerbell.Hardware) (map[string]any, error),
) *renderStore {
	return &renderStore{
		informers: informers,
		mapper:    mapper,
		get:       get,
		resolve:   resolve,
		log:       log,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[types.NamespacedName](),
			workqueue.TypedRateLimitingQueueConfig[types.NamespacedName]{Name: "hardware_render"},
		),
		entries:   map[types.NamespacedName]*renderEntry{},
		refs:      map[types.NamespacedName][]refKey{},
		referrers: map[refKey]map[types.NamespacedName]struct{}{},
		watched:   map[schema.GroupResource]struct{}{},
	}
}

func needsRendering(hw *tinkerbell.Hardware) bool {
	skip, err := hardwareRenderSkip(hw)
	if err != nil {
		return true
	}
	doc, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hw)
	if err != nil {
		return true
	}
	return render.HasTemplates(doc, render.WithSkip(skip))
}

// Start renders every Hardware and keeps renderings current until ctx is done.
func (s *renderStore) Start(ctx context.Context, workers int) error {
	inf, err := s.informers.GetInformer(ctx, &tinkerbell.Hardware{}, cache.BlockUntilSynced(false))
	if err != nil {
		return err
	}
	if _, err := inf.AddEventHandler(s.enqueueHandler(func(key types.NamespacedName) {
		s.queue.Add(key)
	})); err != nil {
		return err
	}

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for s.processNext(ctx) {
			}
		})
	}
	<-ctx.Done()
	s.queue.ShutDown()
	wg.Wait()

	return nil
}

// rendered returns the Hardware-wide result. While a new rendering is pending
// or after it failed, the last successful result is returned. ok is false when
// hw needs rendering and none has succeeded yet.
func (s *renderStore) rendered(hw *tinkerbell.Hardware) (*tinkerbell.Hardware, bool) {
	s.mu.RLock()
	e := s.entries[client.ObjectKeyFromObject(hw)]
	s.mu.RUnlock()

	if e != nil && e.resourceVersion == hw.ResourceVersion && e.err == nil && e.identity {
		return hw, true
	}
	if e == nil || e.resourceVersion != hw.ResourceVersion {
		// Not rendered at this version yet; only the startup and edit windows get here.
		if !needsRendering(hw) {
			return hw, true
		}
	}
	if e != nil && e.good != nil {
		return e.good.DeepCopy(), true
	}
	return nil, false
}

func (s *renderStore) processNext(ctx context.Context) bool {
	key, shutdown := s.queue.Get()
	if shutdown {
		return false
	}
	defer s.queue.Done(key)

	if s.render(ctx, key) {
		s.queue.Forget(key)
	} else {
		s.queue.AddRateLimited(key)
	}
	return true
}

// render renders one Hardware for every consumer and reports whether all
// succeeded.
func (s *renderStore) render(ctx context.Context, key types.NamespacedName) bool {
	hw, err := s.get(ctx, key)
	if apierrors.IsNotFound(err) {
		s.forget(key)
		return true
	}
	if err != nil {
		s.log.Error(err, "get hardware to render", "hardware", key)
		return false
	}
	if !needsRendering(hw) {
		s.track(ctx, key, nil)
		s.mu.Lock()
		s.entries[key] = &renderEntry{resourceVersion: hw.ResourceVersion, good: hw, identity: true}
		s.mu.Unlock()
		return true
	}
	s.track(ctx, key, hw.Spec.References)

	e := &renderEntry{resourceVersion: hw.ResourceVersion}
	// A denied or unreadable reference only fails the render if a template uses it.
	refs, refErr := s.resolve(ctx, hw)
	e.good, err = renderHardware(hw, refs)
	e.identity = e.good == hw
	var servingPrevious bool
	if err != nil {
		e.err = errors.Join(refErr, err)
		e.good, e.identity = nil, false
	}
	s.mu.Lock()
	if err != nil {
		if prev := s.entries[key]; prev != nil {
			e.good = prev.good
			servingPrevious = prev.good != nil
		}
	}
	s.entries[key] = e
	s.mu.Unlock()
	if err != nil {
		s.log.Error(e.err, "render hardware", "hardware", key, "servingPrevious", servingPrevious)
	}

	return err == nil
}

// forget drops everything held for a deleted Hardware.
func (s *renderStore) forget(key types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	s.setRefs(key, nil)
}

// track records what key references, so that a change to a referenced object
// re-renders it, and starts watching referenced types not yet watched.
func (s *renderStore) track(ctx context.Context, key types.NamespacedName, references map[string]tinkerbell.Reference) {
	refs := make([]refKey, 0, len(references))
	for _, r := range references {
		refs = append(refs, refKey{strings.ToLower(r.Group), strings.ToLower(r.Resource), r.Namespace, r.Name})
	}

	s.mu.Lock()
	s.setRefs(key, refs)
	var unwatched []schema.GroupVersionResource
	for _, r := range references {
		gr := schema.GroupResource{Group: strings.ToLower(r.Group), Resource: strings.ToLower(r.Resource)}
		if _, ok := s.watched[gr]; !ok {
			s.watched[gr] = struct{}{}
			unwatched = append(unwatched, gr.WithVersion(r.Version))
		}
	}
	s.mu.Unlock()

	for _, gvr := range unwatched {
		if err := s.watch(ctx, gvr); err != nil {
			s.log.Error(err, "watch referenced objects; retrying on the next render", "resource", gvr)
			s.mu.Lock()
			delete(s.watched, gvr.GroupResource())
			s.mu.Unlock()
		}
	}
}

// setRefs replaces key's references in the reverse index. s.mu must be held.
func (s *renderStore) setRefs(key types.NamespacedName, refs []refKey) {
	for _, r := range s.refs[key] {
		delete(s.referrers[r], key)
		if len(s.referrers[r]) == 0 {
			delete(s.referrers, r)
		}
	}
	if len(refs) == 0 {
		delete(s.refs, key)
		return
	}
	s.refs[key] = refs
	for _, r := range refs {
		if s.referrers[r] == nil {
			s.referrers[r] = map[types.NamespacedName]struct{}{}
		}
		s.referrers[r][key] = struct{}{}
	}
}

// watch starts a metadata-only informer on gvr that re-renders the Hardware
// referencing a changed object.
func (s *renderStore) watch(ctx context.Context, gvr schema.GroupVersionResource) error {
	gvk, err := s.mapper.KindFor(gvr)
	if err != nil {
		return err
	}
	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(gvk)
	inf, err := s.informers.GetInformer(ctx, obj, cache.BlockUntilSynced(false))
	if err != nil {
		return err
	}
	_, err = inf.AddEventHandler(s.enqueueHandler(func(changed types.NamespacedName) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for hw := range s.referrers[refKey{gvr.Group, gvr.Resource, changed.Namespace, changed.Name}] {
			s.queue.Add(hw)
		}
	}))
	return err
}

// enqueueHandler calls enqueue with the key of every added, updated or deleted object.
func (s *renderStore) enqueueHandler(enqueue func(types.NamespacedName)) toolscache.ResourceEventHandler {
	handle := func(obj any) {
		if t, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
			obj = t.Obj
		}
		if o, ok := obj.(client.Object); ok {
			enqueue(client.ObjectKeyFromObject(o))
		}
	}
	return toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { handle(obj) },
		UpdateFunc: func(_, obj any) { handle(obj) },
		DeleteFunc: handle,
	}
}
