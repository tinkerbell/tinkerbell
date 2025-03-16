package crd

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func TestXxx(t *testing.T) {
	var curCRDs sync.Map
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "customresourcedefinitions", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		// get the CRD object from the action
		a, ok := action.(ktesting.CreateAction)
		if !ok {
			return false, nil, fmt.Errorf("unexpected action type: %T", action)
		}
		// add the status conditions to the CRD object
		o, ok := a.GetObject().(*v1.CustomResourceDefinition)
		if !ok {
			return false, nil, fmt.Errorf("unexpected object type: %T", a.GetObject())
		}
		o.Status.Conditions = []v1.CustomResourceDefinitionCondition{
			{
				Type:   v1.Established,
				Status: v1.ConditionTrue,
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
			},
			{
				Type:   v1.NamesAccepted,
				Status: v1.ConditionTrue,
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
			},
		}

		curCRDs.Store(o.Name, o)
		return true, o, nil
	})
	client.PrependReactor("get", "customresourcedefinitions", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		a, ok := action.(ktesting.GetAction)
		if ok {
			if crd, ok := curCRDs.Load(a.GetName()); ok {
				return true, crd.(*v1.CustomResourceDefinition), nil
			}
		}
		return false, nil, nil
	})
	m := NewTinkerbell(WithClient(client))
	if err := m.MigrateAndValidate(context.Background()); err != nil {
		t.Errorf("failed to migrate CRDs: %v", err)
	}
}
