package kube

import (
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type hardwareNotFoundError struct {
	name      string
	namespace string
}

func (h hardwareNotFoundError) NotFound() bool { return true }

func (h hardwareNotFoundError) Error() string {
	return fmt.Sprintf("hardware not found: %v, namespace: %v", h.name, h.namespace)
}

// Status() implements the APIStatus interface from apimachinery/pkg/api/errors
// so that IsNotFound function could be used against this error type.
func (h hardwareNotFoundError) Status() metav1.Status {
	return metav1.Status{
		Reason: metav1.StatusReasonNotFound,
		Code:   http.StatusNotFound,
	}
}

type foundMultipleHardwareError struct {
	name      string
	namespace string
	count     int
}

func (f foundMultipleHardwareError) MultipleFound() bool { return true }

func (f foundMultipleHardwareError) Error() string {
	return fmt.Sprintf("found %d hardware objects for name: %s, namespace: %s", f.count, f.name, f.namespace)
}
