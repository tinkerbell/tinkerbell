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

func (f *fakeDynamicClient) Resource(_ schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return f
}

func (f *fakeDynamicClient) Apply(_ context.Context, _ string, _ *unstructured.Unstructured, _ metav1.ApplyOptions, _ ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeDynamicClient) ApplyStatus(_ context.Context, _ string, _ *unstructured.Unstructured, _ metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeDynamicClient) Create(_ context.Context, _ *unstructured.Unstructured, _ metav1.CreateOptions, _ ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeDynamicClient) Update(_ context.Context, _ *unstructured.Unstructured, _ metav1.UpdateOptions, _ ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeDynamicClient) UpdateStatus(_ context.Context, _ *unstructured.Unstructured, _ metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeDynamicClient) Delete(_ context.Context, _ string, _ metav1.DeleteOptions, _ ...string) error {
	return nil
}

func (f *fakeDynamicClient) DeleteCollection(_ context.Context, _ metav1.DeleteOptions, _ metav1.ListOptions) error {
	return nil
}

func (f *fakeDynamicClient) Get(_ context.Context, name string, _ metav1.GetOptions, _ ...string) (*unstructured.Unstructured, error) {
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

func (f *fakeDynamicClient) List(_ context.Context, _ metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, nil
}

func (f *fakeDynamicClient) Namespace(_ string) dynamic.ResourceInterface {
	return f
}

func (f *fakeDynamicClient) Patch(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (f *fakeDynamicClient) Watch(_ context.Context, _ metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}
