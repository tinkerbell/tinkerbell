package controllers_test

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
	rufiov1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/rufio/controllers"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// This source file is currently a bucket of stuff. If it grows too big, consider breaking it
// into more granular helper sources.

// createKubeClientBuilder creates a fake kube client builder loaded with Rufio's and Kubernetes'
// corev1 schemes.
func createKubeClientBuilder() *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	if err := rufiov1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	return fake.NewClientBuilder().
		WithScheme(scheme)
}

// createKubeClientWithObjects creates a kubernetes client with the given objects.
func createKubeClientWithObjects(objects ...client.Object) client.WithWatch {
	return createKubeClientBuilder().
		WithObjects(objects...).
		Build()
}

// createKubeClientWithObjectsForJobController creates a kubernetes client with the given objects
// and the indexes required by the Job controller.
func createKubeClientWithObjectsForJobController(objects ...client.Object) client.WithWatch {
	return createKubeClientBuilder().
		WithObjects(objects...).
		WithIndex(&bmcv1alpha1.Task{}, ".metadata.controller", controllers.TaskOwnerIndexFunc).
		Build()
}

// mustCreateLogr creates a logr.Logger implementation backed by a debug style sink. It panics
// if the logger fails to create.
func mustCreateLogr(name ...string) logr.Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	if len(name) == 1 {
		logger = logger.Named(name[0])
	}

	return zapr.NewLogger(logger)
}
