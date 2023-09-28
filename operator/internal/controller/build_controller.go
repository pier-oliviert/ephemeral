/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	tasks "github.com/releasehub-com/spot/operator/internal/tasks/builds"
)

var ErrStageWithInvalidState = errors.New("stage did not match the status of the build")
var ErrPodUnexpectlyFailed = errors.New("pod failed without notifying the build")

const (
	kPodStatusField = ".status.pod"
)

// BuildReconciler reconciles a Build object
type BuildReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	record.EventRecorder
}

//+kubebuilder:rbac:groups=spot.release.com,resources=builds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=spot.release.com,resources=builds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=spot.release.com,resources=builds/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;watch;list;create;delete

func (r *BuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var build spot.Build
	if err := r.Client.Get(ctx, req.NamespacedName, &build); err != nil {
		if k8sErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Couldn't retrieve the build", "NamespacedName", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Let's first see if one of the existing condition has failed since that would be unrecoverable
	// and the reconcilation can be done with this build.
	if build.Status.Conditions.Phase() == spot.BuildPhaseError {
		return ctrl.Result{}, nil
	}

	if condition := build.Status.GetCondition(spot.BuildConditionDeployPod); condition.Status == spot.ConditionInitialized {
		pd := tasks.PodDeployment{Client: r.Client, EventRecorder: r.EventRecorder}
		result, err := pd.Reconcile(ctx, &build, &condition)
		if err != nil {
			return result, r.markBuildHasErrored(ctx, &build, err)
		}

		return result, nil
	}

	if build.Status.Conditions.Phase() == spot.BuildPhaseRunning {
		// Most of the lifecycle of a Build CRD is deferred to the pod that was created during the initialization
		// process. The only thing to watch out for here is to make sure the pod is either scheduled to run, or is running & healthy.
		// If, for any reason, the pod is dead, the reconciler needs to mark this build as failed.

		var pod core.Pod
		err := r.Get(ctx, build.Status.Pod.NamespacedName(), &pod)
		if err != nil {
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
				condition := build.Status.GetCondition(spot.BuildConditionDeployPod)
				condition.Status = spot.ConditionError
				build.Status.SetCondition(condition)
				return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, ErrPodUnexpectlyFailed)
			}
		}
	}

	if build.Status.Conditions.Phase() == spot.BuildPhaseDone {
		// The build was successful, since the pod was in charge of maintaining the state of this
		// custom resource, there isn't anything for the build to do beside doing some housekeeping.
		// The pod doesn't need to exist anymore.
		var pod core.Pod
		if err := r.Client.Get(ctx, build.Status.Pod.NamespacedName(), &pod); err != nil {
			if k8sErrors.IsNotFound(err) {
				// Pod was already deleted, can safely return
				return ctrl.Result{}, nil
			}

			// Error is not of type ErrNotFound, can't recover from this
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}

		r.EventRecorder.Event(&build, "Normal", string(build.Status.Phase), fmt.Sprintf("Clearing the builder pod(%s/%s)", pod.Namespace, pod.Name))

		if err := r.Client.Delete(ctx, &pod); err != nil {
			r.EventRecorder.Event(&build, "Warning", string(build.Status.Phase), fmt.Sprintf("Could not delete the pod as part of housekeeping, pod: %s/%s", pod.Namespace, pod.Name))
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &spot.Build{}, kPodStatusField, func(rawObj client.Object) []string {
		build := rawObj.(*spot.Build)
		pod := build.Status.Pod
		if pod == nil {
			return nil
		}

		return []string{pod.Namespace, pod.Name}
	})

	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&spot.Build{}).
		Watches(
			&core.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueBuildReconcilerForOwnedPod),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// This Map Function takes a client.Object (a core.Pod, in this case) and returns a list of reconcile.Request for
// any pod that is associated with a Build custom resource. The goal of this enqueuRequest func is not to be too smart
// about the state of the pod or the state of the build but rather gather just gather all build that can be found and
// let the reconciler figure the small details itself.
//
// By nature of the API, the logic here handles all objects as slices, but it's just because these API call *could*
// return more than 1 resource for each. As it currently stand, it's to be expected that all items fetched that returns
// a slice returns a slice of length=1.
func (r *BuildReconciler) enqueueBuildReconcilerForOwnedPod(ctx context.Context, pod client.Object) []reconcile.Request {
	builds := &spot.BuildList{}

	// Retrieve a list of all the builds that matches the pod's
	err := r.List(ctx, builds, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(kPodStatusField, pod.GetName()),
		Namespace:     pod.GetNamespace(),
	})

	if err != nil {
		return []reconcile.Request{}
	}

	// As described above, it's expected for the build items to be of length = 1.
	requests := make([]reconcile.Request, len(builds.Items))
	for i, item := range builds.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}

	// These requests are what's used internally by K8S to call the Reconciler of this CRD.
	return requests
}

func (r *BuildReconciler) markBuildHasErrored(ctx context.Context, build *spot.Build, err error) error {
	r.EventRecorder.Event(build, "Warning", string(spot.BuildPhaseError), err.Error())
	build.Status.Phase = spot.BuildPhaseError
	return r.Client.Status().Update(ctx, build)
}
