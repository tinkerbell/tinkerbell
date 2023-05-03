package controllers_test

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/rufio/controllers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestJobReconciler_TasklessJob(t *testing.T) {
	g := gomega.NewWithT(t)

	machine := createMachine()
	secret := createSecret()
	job := createJob("test", machine)

	kubeClient := createKubeClientWithObjectsForJobController(machine, secret, job)

	reconciler := controllers.NewJobReconciler(kubeClient)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: job.Namespace,
			Name:      job.Name,
		},
	}

	result, err := reconciler.Reconcile(context.Background(), request)
	g.Expect(err).To(gomega.Succeed())
	g.Expect(result).To(gomega.Equal(reconcile.Result{}))

	var retrieved bmcv1alpha1.Job
	err = kubeClient.Get(context.Background(), request.NamespacedName, &retrieved)
	g.Expect(err).To(gomega.Succeed())
}

func TestJobReconciler_UnknownMachine(t *testing.T) {
	g := gomega.NewWithT(t)

	secret := createSecret()
	job := &bmcv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: bmcv1alpha1.JobSpec{
			MachineRef: bmcv1alpha1.MachineRef{Name: "unknown", Namespace: "default"},
			Tasks:      []bmcv1alpha1.Action{},
		},
	}

	builder := createKubeClientBuilder()
	builder.WithObjects(secret, job)
	kubeClient := builder.Build()

	reconciler := controllers.NewJobReconciler(kubeClient)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: job.Namespace,
			Name:      job.Name,
		},
	}

	_, err := reconciler.Reconcile(context.Background(), request)
	g.Expect(err).To(gomega.HaveOccurred())
}

func TestJobReconciler_Reconcile(t *testing.T) {
	for name, action := range map[string]bmcv1alpha1.Action{
		"PowerAction": {PowerAction: bmcv1alpha1.PowerOn.Ptr()},
		"OneTimeBootDeviceAction": {
			OneTimeBootDeviceAction: &bmcv1alpha1.OneTimeBootDeviceAction{
				Devices: []bmcv1alpha1.BootDevice{bmcv1alpha1.PXE},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			machine := createMachine()
			secret := createSecret()
			job := createJob(name, machine)
			job.Spec.Tasks = append(job.Spec.Tasks, action)

			cluster := createKubeClientWithObjectsForJobController(machine, secret, job)

			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: job.Namespace,
					Name:      job.Name,
				},
			}

			reconciler := controllers.NewJobReconciler(cluster)
			result, err := reconciler.Reconcile(context.Background(), request)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(result).To(gomega.Equal(reconcile.Result{}))

			var retrieved1 bmcv1alpha1.Job
			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved1)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved1.Status.StartTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 10))
			g.Expect(retrieved1.Status.CompletionTime.IsZero()).To(gomega.BeTrue())
			g.Expect(retrieved1.Status.Conditions).To(gomega.HaveLen(1))
			g.Expect(retrieved1.Status.Conditions[0].Type).To(gomega.Equal(bmcv1alpha1.JobRunning))
			g.Expect(retrieved1.Status.Conditions[0].Status).To(gomega.Equal(bmcv1alpha1.ConditionTrue))

			var task bmcv1alpha1.Task
			taskKey := types.NamespacedName{
				Namespace: job.Namespace,
				Name:      bmcv1alpha1.FormatTaskName(*job, 0),
			}
			err = cluster.Get(context.Background(), taskKey, &task)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(task.Spec.Task).To(gomega.BeEquivalentTo(job.Spec.Tasks[0]))
			g.Expect(task.OwnerReferences).To(gomega.HaveLen(1))
			g.Expect(task.OwnerReferences[0].Name).To(gomega.Equal(job.Name))
			g.Expect(task.OwnerReferences[0].Kind).To(gomega.Equal("Job"))

			// Ensure re-reconciling a job does nothing given the task is still outstanding.
			result, err = reconciler.Reconcile(context.Background(), request)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(result).To(gomega.Equal(reconcile.Result{}))

			var retrieved2 bmcv1alpha1.Job
			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved2)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved2).To(gomega.BeEquivalentTo(retrieved1))
		})
	}
}

func createJob(name string, machine *bmcv1alpha1.Machine) *bmcv1alpha1.Job {
	return &bmcv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: bmcv1alpha1.JobSpec{
			MachineRef: bmcv1alpha1.MachineRef{Name: machine.Name, Namespace: machine.Namespace},
			Tasks:      []bmcv1alpha1.Action{},
		},
	}
}
