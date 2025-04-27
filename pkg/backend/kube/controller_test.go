package kube

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

func TestDynamicRead(t *testing.T) {
	tests := map[string]struct {
		wantObj map[string]interface{}
		error   error
	}{
		"success": {
			wantObj: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "test-deployment",
					"namespace": "default",
					"labels": map[string]string{
						"app": "test-app",
					},
				},
			},
		},
		"error": {error: errors.New("error getting resource")},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			b := &Backend{
				DynamicClient: &fakeDynamicClient{
					gvr: schema.GroupVersionResource{
						Group:    "apps",
						Version:  "v1",
						Resource: "deployments",
					},
					namespace: "default",
					error:     tt.error,
				},
			}

			gvr := schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			}
			name := "test-deployment"
			namespace := "default"

			object, err := b.DynamicRead(ctx, gvr, name, namespace)
			if tt.error != nil && err == nil {
				t.Fatalf("expected error: %v, got nil", err)
			}
			if tt.error == nil && err != nil {
				t.Fatalf("expected nil, got error: %v", err)
			}

			if diff := cmp.Diff(object, tt.wantObj); diff != "" {
				t.Fatalf("expected %v, got %v", tt.wantObj, object)
			}
		})
	}
}

type fakeDynamicClient struct {
	gvr       schema.GroupVersionResource
	namespace string
	error     error
}

func (f *fakeDynamicClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return f
}

func (f *fakeDynamicClient) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, options metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (f *fakeDynamicClient) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, options metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (f *fakeDynamicClient) Create(ctx context.Context, obj *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (f *fakeDynamicClient) Update(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (f *fakeDynamicClient) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (f *fakeDynamicClient) Delete(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
	return nil
}
func (f *fakeDynamicClient) DeleteCollection(ctx context.Context, options metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}

func (f *fakeDynamicClient) Get(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": f.gvr.Group + "/" + f.gvr.Version,
			"kind":       cases.Title(language.English, cases.NoLower).String((strings.TrimSuffix(f.gvr.Resource, "s"))),
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": f.namespace,
				"labels": map[string]string{
					"app": "test-app",
				},
			},
		},
	}, f.error
}
func (f *fakeDynamicClient) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": f.gvr.Group + "/" + f.gvr.Version,
					"kind":       f.gvr.Resource,
					"metadata": map[string]interface{}{
						"name":      "test-deployment",
						"namespace": f.namespace,
						"labels": map[string]string{
							"app": "test-app",
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": f.gvr.Group + "/" + f.gvr.Version,
					"kind":       f.gvr.Resource,
					"metadata": map[string]interface{}{
						"name":      "test-deployment-2",
						"namespace": f.namespace,
						"labels": map[string]string{
							"app": "test-app-2",
						},
					},
				},
			},
		},
	}, nil
}

func (f *fakeDynamicClient) Namespace(namespace string) dynamic.ResourceInterface {
	return f
}

func (f *fakeDynamicClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (f *fakeDynamicClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}
