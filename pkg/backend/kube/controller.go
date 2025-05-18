package kube

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DynamicRead reads any Kubernetes resource, defined via gvr, name, and namespace, and returns the spec field
// as a map[string]interface{}. It uses the Kubernetes dynamic client to perform the read operation.
func (b *Backend) DynamicRead(ctx context.Context, gvr schema.GroupVersionResource, name, namespace string) (map[string]interface{}, error) {
	// Here's the spec (https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#resources):
	// Resource collections should be all lowercase and plural, [...], Group names must be lower case and be valid DNS subdomains.
	sanitizedGVR := schema.GroupVersionResource{
		Group:    strings.ToLower(gvr.Group),
		Version:  gvr.Version,
		Resource: strings.ToLower(gvr.Resource),
	}
	res := b.DynamicClient.Resource(sanitizedGVR).Namespace(namespace)
	one, err := res.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting resource: %w", err)
	}

	return one.Object, nil
}
