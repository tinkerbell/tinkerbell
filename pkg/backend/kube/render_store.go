package kube

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/template/render"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
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
	informers          informerGetter
	referenceInformers metadataInformerFactory
	mapper             meta.RESTMapper
	get                func(context.Context, types.NamespacedName) (*tinkerbell.Hardware, error)
	resolve            func(context.Context, *tinkerbell.Hardware) (map[string]any, error)
	queue              workqueue.TypedRateLimitingInterface[types.NamespacedName]
	log                logr.Logger
	renders            *prometheus.CounterVec
	duration           prometheus.Histogram
	fallbacks          prometheus.Gauge
	notify             chan struct{}
	changes            map[types.NamespacedName]struct{}

	mu        sync.RWMutex
	entries   map[types.NamespacedName]*renderEntry
	decisions map[types.NamespacedName]renderDecision
	refs      map[types.NamespacedName][]refKey
	referrers map[refKey]map[types.NamespacedName]struct{}
	watchMu   sync.Mutex
	watched   map[schema.GroupResource]struct{}
}

type informerGetter interface {
	GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error)
}

type metadataInformerFactory interface {
	ForResource(schema.GroupVersionResource) informers.GenericInformer
	Start(<-chan struct{})
	Shutdown()
}

type metadataListClient struct {
	metadata.Interface
	listed func(schema.GroupResource)
}

func (c *metadataListClient) Resource(resource schema.GroupVersionResource) metadata.Getter {
	getter := c.Interface.Resource(resource)
	return &metadataListResource{ResourceInterface: getter, namespace: getter.Namespace, listed: func() { c.listed(resource.GroupResource()) }}
}

// Explicit LIST/WATCH keeps every completed snapshot on the reconciliation path.
func (*metadataListClient) IsWatchListSemanticsUnSupported() bool { return true }

type metadataListResource struct {
	metadata.ResourceInterface
	namespace func(string) metadata.ResourceInterface
	listed    func()
}

func (r *metadataListResource) Namespace(namespace string) metadata.ResourceInterface {
	scoped := *r
	scoped.ResourceInterface = r.namespace(namespace)
	return &scoped
}

func (r *metadataListResource) List(ctx context.Context, options metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
	list, err := r.ResourceInterface.List(ctx, options)
	if err == nil {
		r.listed()
	}
	return list, err
}

// renderEntry is replaced whole, never modified, so readers need no lock on it.
type renderEntry struct {
	uid             types.UID            // of the Hardware owning this entry
	resourceVersion string               // of the Hardware last rendered
	err             error                // rendering that resourceVersion failed
	good            *tinkerbell.Hardware // last successful rendering, nil if none
	identity        bool                 // good is the stored object: nothing needed rendering
}

type renderDecision struct {
	uid             types.UID
	resourceVersion string
	needed          bool
}

type refKey struct {
	group, resource, namespace, name string
}

func newRenderStore(
	log logr.Logger,
	hardwareInformers informerGetter,
	mapper meta.RESTMapper,
	metadataClient metadata.Interface,
	get func(context.Context, types.NamespacedName) (*tinkerbell.Hardware, error),
	resolve func(context.Context, *tinkerbell.Hardware) (map[string]any, error),
	registry prometheus.Registerer,
) *renderStore {
	store := &renderStore{
		informers: hardwareInformers,
		mapper:    mapper,
		get:       get,
		resolve:   resolve,
		log:       log,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[types.NamespacedName](),
			workqueue.TypedRateLimitingQueueConfig[types.NamespacedName]{Name: "hardware_render"},
		),
		entries:   map[types.NamespacedName]*renderEntry{},
		decisions: map[types.NamespacedName]renderDecision{},
		refs:      map[types.NamespacedName][]refKey{},
		referrers: map[refKey]map[types.NamespacedName]struct{}{},
		watched:   map[schema.GroupResource]struct{}{},
		notify:    make(chan struct{}, 1),
		changes:   map[types.NamespacedName]struct{}{},
	}
	store.referenceInformers = metadatainformer.NewSharedInformerFactory(&metadataListClient{Interface: metadataClient, listed: store.requeueResource}, 0)
	store.renders = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tinkerbell_hardware_render_attempts_total", Help: "Hardware render attempts by outcome."}, []string{"result"})
	store.duration = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "tinkerbell_hardware_render_duration_seconds", Help: "Hardware render attempt duration."})
	store.fallbacks = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tinkerbell_hardware_render_fallback_entries", Help: "Hardware entries serving a previous result after a render failure."})
	registry.MustRegister(store.renders, store.duration, store.fallbacks,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "tinkerbell_hardware_render_queue_depth", Help: "Hardware keys waiting for rendering."}, func() float64 { return float64(store.queue.Len()) }))
	return store
}

func (s *renderStore) Changes() <-chan struct{} { return s.notify }

func (s *renderStore) TakeChanges() []types.NamespacedName {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]types.NamespacedName, 0, len(s.changes))
	for key := range s.changes {
		keys = append(keys, key)
	}
	clear(s.changes)
	return keys
}

func (s *renderStore) publishLocked(key types.NamespacedName, entry *renderEntry) {
	if previous := s.entries[key]; previous != nil && previous.err != nil && previous.good != nil {
		s.fallbacks.Dec()
	}
	if entry == nil {
		delete(s.entries, key)
	} else {
		s.entries[key] = entry
		if entry.err != nil && entry.good != nil {
			s.fallbacks.Inc()
		}
	}
	s.changes[key] = struct{}{}
	select {
	case s.notify <- struct{}{}:
	default:
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

func (s *renderStore) rememberDecision(hw *tinkerbell.Hardware) bool {
	decision := renderDecision{uid: hw.UID, resourceVersion: hw.ResourceVersion, needed: needsRendering(hw)}
	s.mu.Lock()
	s.decisions[client.ObjectKeyFromObject(hw)] = decision
	s.mu.Unlock()
	return decision.needed
}

// Start renders every Hardware and keeps renderings current until ctx is done.
func (s *renderStore) Start(ctx context.Context, workers int) error {
	defer close(s.notify)
	defer s.queue.ShutDown()
	inf, err := s.informers.GetInformer(ctx, &tinkerbell.Hardware{}, cache.BlockUntilSynced(false))
	if err != nil {
		return err
	}
	if _, err := inf.AddEventHandler(s.hardwareHandler()); err != nil {
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
	s.referenceInformers.Shutdown()

	return nil
}

// rendered returns the Hardware-wide result. While a new rendering is pending
// or after it failed, the last successful result is returned. ok is false when
// hw needs rendering and none has succeeded yet.
func (s *renderStore) rendered(hw *tinkerbell.Hardware) (*tinkerbell.Hardware, bool) {
	s.mu.RLock()
	e := s.entries[client.ObjectKeyFromObject(hw)]
	decision, known := s.decisions[client.ObjectKeyFromObject(hw)]
	s.mu.RUnlock()
	if e != nil && e.uid != hw.UID {
		e = nil
	}

	if e != nil && e.resourceVersion == hw.ResourceVersion && e.err == nil && e.identity {
		return hw, true
	}
	if e == nil || e.resourceVersion != hw.ResourceVersion {
		// Not rendered at this version yet; only the startup and edit windows get here.
		needed := decision.needed
		if !known || decision.uid != hw.UID || decision.resourceVersion != hw.ResourceVersion {
			needed = needsRendering(hw)
		}
		if !needed {
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

// render reports whether rendering and reference watch registration succeeded.
func (s *renderStore) render(ctx context.Context, key types.NamespacedName) (succeeded bool) {
	started := time.Now()
	defer func() {
		result := "success"
		if !succeeded {
			result = "error"
		}
		s.renders.WithLabelValues(result).Inc()
		s.duration.Observe(time.Since(started).Seconds())
	}()
	hw, err := s.get(ctx, key)
	if apierrors.IsNotFound(err) {
		s.forget(key)
		return true
	}
	if err != nil {
		s.log.Error(err, "get hardware to render", "hardware", key)
		return false
	}
	if !s.rememberDecision(hw) {
		s.mu.Lock()
		s.setRefs(key, nil)
		s.publishLocked(key, &renderEntry{uid: hw.UID, resourceVersion: hw.ResourceVersion, good: hw, identity: true})
		s.mu.Unlock()
		return true
	}
	watchErr := s.track(ctx, key, hw.Spec.References)

	e := &renderEntry{uid: hw.UID, resourceVersion: hw.ResourceVersion}
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
		if prev := s.entries[key]; prev != nil && prev.uid == hw.UID {
			e.good = prev.good
			servingPrevious = prev.good != nil
		}
	}
	s.publishLocked(key, e)
	s.mu.Unlock()
	if err != nil {
		s.log.Error(e.err, "render hardware", "hardware", key, "servingPrevious", servingPrevious)
	}

	return err == nil && watchErr == nil
}

// forget drops everything held for a deleted Hardware.
func (s *renderStore) forget(key types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishLocked(key, nil)
	delete(s.decisions, key)
	s.setRefs(key, nil)
}

// track records what key references, so that a change to a referenced object
// re-renders it, and starts watching referenced types not yet watched.
func (s *renderStore) track(ctx context.Context, key types.NamespacedName, references map[string]tinkerbell.Reference) error {
	refs := make([]refKey, 0, len(references))
	for _, r := range references {
		refs = append(refs, refKey{strings.ToLower(r.Group), strings.ToLower(r.Resource), r.Namespace, r.Name})
	}

	s.mu.Lock()
	s.setRefs(key, refs)
	s.mu.Unlock()

	var watchErr error
	for _, reference := range references {
		gvr := schema.GroupVersionResource{Group: strings.ToLower(reference.Group), Resource: strings.ToLower(reference.Resource), Version: reference.Version}
		if err := s.ensureWatch(ctx, gvr); err != nil {
			s.log.Error(err, "watch referenced objects; scheduling retry", "resource", gvr)
			watchErr = errors.Join(watchErr, err)
		}
	}
	return watchErr
}

func (s *renderStore) requeueResource(resource schema.GroupResource) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for reference, hardware := range s.referrers {
		if reference.group == resource.Group && reference.resource == resource.Resource {
			for key := range hardware {
				s.queue.Add(key)
			}
		}
	}
}

func (s *renderStore) ensureWatch(ctx context.Context, gvr schema.GroupVersionResource) error {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if _, ok := s.watched[gvr.GroupResource()]; ok {
		return nil
	}
	if err := s.watch(ctx, gvr); err != nil {
		return err
	}
	s.watched[gvr.GroupResource()] = struct{}{}
	return nil
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
	if _, err := s.mapper.KindFor(gvr); err != nil {
		return err
	}
	inf := s.referenceInformers.ForResource(gvr).Informer()
	_, err := inf.AddEventHandler(s.enqueueHandler(func(changed types.NamespacedName) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for hw := range s.referrers[refKey{gvr.Group, gvr.Resource, changed.Namespace, changed.Name}] {
			s.queue.Add(hw)
		}
	}))
	if err == nil {
		s.referenceInformers.Start(ctx.Done())
	}
	return err
}

// enqueueHandler calls enqueue with the key of every added, updated or deleted object.
func (s *renderStore) hardwareHandler() toolscache.ResourceEventHandler {
	handler := s.enqueueHandler(func(key types.NamespacedName) { s.queue.Add(key) })
	enqueue := handler.AddFunc
	handler.AddFunc = func(object any) {
		if hw, ok := object.(*tinkerbell.Hardware); ok {
			s.rememberDecision(hw)
		}
		enqueue(object)
	}
	handler.UpdateFunc = func(before, after any) {
		oldObject, oldOK := before.(client.Object)
		newObject, newOK := after.(client.Object)
		changed := !oldOK || !newOK || hardwareInputsChanged(oldObject, newObject)
		if changed {
			handler.AddFunc(after)
		} else if hw, ok := after.(*tinkerbell.Hardware); ok {
			s.rememberDecision(hw)
		}
	}
	return handler
}

func hardwareInputsChanged(before, after client.Object) bool {
	documents := make([]map[string]any, 0, 2)
	for _, object := range []client.Object{before, after} {
		document, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			return true
		}
		document = runtime.DeepCopyJSON(document)
		unstructured.RemoveNestedField(document, "metadata", "resourceVersion")
		unstructured.RemoveNestedField(document, "metadata", "managedFields")
		value, found, err := unstructured.NestedFieldNoCopy(document, "status", "conditions")
		if err != nil {
			return true
		}
		if found {
			conditions, ok := value.([]any)
			if !ok {
				return true
			}
			var remaining []any
			for _, condition := range conditions {
				fields, ok := condition.(map[string]any)
				if !ok {
					return true
				}
				if fields["type"] != "Rendered" {
					remaining = append(remaining, condition)
				}
			}
			if len(remaining) == 0 {
				unstructured.RemoveNestedField(document, "status", "conditions")
			} else if err := unstructured.SetNestedSlice(document, remaining, "status", "conditions"); err != nil {
				return true
			}
		}
		if status, ok := document["status"].(map[string]any); ok && len(status) == 0 {
			delete(document, "status")
		}
		documents = append(documents, document)
	}
	return !reflect.DeepEqual(documents[0], documents[1])
}

func (s *renderStore) enqueueHandler(enqueue func(types.NamespacedName)) toolscache.ResourceEventHandlerFuncs {
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
