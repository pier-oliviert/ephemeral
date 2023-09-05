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
	"k8s.io/apimachinery/pkg/api/resource"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/env"

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
		// A build error means the whole workspace can't progress further. Let's notify workspace and call it.
		var workspace spot.Workspace
		var reference *meta.OwnerReference
		for _, ref := range build.ObjectMeta.OwnerReferences {
			if ref.Kind == "Workspace" {
				reference = &ref
				break
			}
		}

		if reference == nil {
			// No reference exists, create an event that notes that this build didn't belong to any workspace and be done with it.
			r.EventRecorder.Event(&build, "Normal", string(build.Status.Phase), "No workspace to notify: none owns this build")
			return ctrl.Result{}, nil
		}

		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: build.Namespace, Name: reference.Name}, &workspace); err != nil {
			return ctrl.Result{Requeue: false}, r.markBuildHasErrored(ctx, &build, err)
		}

		// TODO: Workspace CRD should watch for builds and should update
		// its own stage.
		workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
			Type:   spot.WorkspaceConditionImages,
			Status: spot.ConditionError,
		})
		if err := r.Client.SubResource("status").Update(ctx, &workspace); err != nil {
			logger.Error(err, "fatal error updating the workspace status")
		}

		return ctrl.Result{Requeue: false}, r.markBuildHasErrored(ctx, &build, errors.New("image could not be built"))
	}

	if condition := build.Status.GetCondition(spot.BuildConditionDeployPod); condition.Status == spot.ConditionInitialized {
		// The Build is just initialized and nothing has been processed, yet. For the Build to actually start, a pod
		// needs to be scheduled with the right service account so that it can update the state of the Build has it goes
		// through each of the steps.
		logger.Info("Launching pod")
		pod, err := r.buildPod(ctx, &build)
		if err != nil {
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}

		// It's important to set the condition first before calling conditions.Phase() as otherwise it would
		// not include the state of this condition when deriving the value.
		build.Status.SetCondition(spot.BuildCondition{
			Type:   spot.BuildConditionDeployPod,
			Status: spot.ConditionInProgress,
		})

		build.Status.Pod = spot.NewReference(pod)
		build.Status.Phase = build.Status.Conditions.Phase()

		if err := r.Client.Status().Update(ctx, &build); err != nil {
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}
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

		logger.Info("Pod", "Status", pod.Status)

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
		// The build was successful and the pod that ran the build has completed. Let's update the status on
		// the Workspace now that a build for that workspace is done.
		var workspace spot.Workspace
		var reference *meta.OwnerReference
		for _, ref := range build.ObjectMeta.OwnerReferences {
			if ref.Kind == "Workspace" {
				reference = &ref
				break
			}
		}

		if reference == nil {
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, ErrStageWithInvalidState)
		}

		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: build.Namespace, Name: reference.Name}, &workspace); err != nil {
			return ctrl.Result{Requeue: false}, r.markBuildHasErrored(ctx, &build, err)
		}

		if workspace.Status.Images == nil {
			// This build is the first to add an entry, make the map
			workspace.Status.Images = make(map[string]spot.BuildImage)
		}

		// Update workspace with the Image from the build
		workspace.Status.Images[fmt.Sprintf("%s:%s", build.ImageURL(), r.tagFor(&build))] = *build.Status.Image
		if err := r.Client.SubResource("status").Update(ctx, &workspace); err != nil {
			// Can't update the workspace with this build's information.
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}

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

func (r *BuildReconciler) buildPod(ctx context.Context, build *spot.Build) (*core.Pod, error) {
	privileged := true
	pod := &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Namespace:    build.Namespace,
			GenerateName: fmt.Sprintf("build-%s-", build.Name),
			Annotations:  map[string]string{},
			OwnerReferences: []meta.OwnerReference{
				{
					APIVersion: build.APIVersion,
					Kind:       build.Kind,
					Name:       build.Name,
					UID:        build.UID,
				},
			},
		},
		Spec: core.PodSpec{
			RestartPolicy:      core.RestartPolicyNever,
			ServiceAccountName: "spot-controller-manager", // TODO: Most likely to change spot-system/default to support the RBAC settings we need instead
			Containers: []core.Container{{
				Name:            "buildkit",
				Image:           env.GetString("BUILDER_IMAGE", "builder:dev"),
				ImagePullPolicy: core.PullNever,
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
					Limits: core.ResourceList{
						"memory": resource.MustParse("2Gi"),
					},
				},
				Env: []core.EnvVar{
					{
						Name:  "BUILD_REFERENCE",
						Value: build.GetReference().String(),
					},
					{
						Name:  "REPOSITORY_URL",
						Value: build.Spec.Image.Repository.URL,
					},
					{
						Name:  "REPOSITORY_REF",
						Value: build.Spec.Image.Repository.Ref,
					},
					{
						Name:  "IMAGE_URL",
						Value: build.ImageURL(),
					},
					{
						Name:  "IMAGE_TAG",
						Value: r.tagFor(build),
					},
				},
				SecurityContext: &core.SecurityContext{
					Privileged: &privileged,
				},
				VolumeMounts: []core.VolumeMount{{
					Name:      "buildkit-socket",
					MountPath: "/run/buildkit/",
				}},
			},
				{
					Name:  "buildkitd",
					Image: "moby/buildkit:master",
					Resources: core.ResourceRequirements{
						Requests: core.ResourceList{
							"memory": resource.MustParse("1Gi"),
						},
						Limits: core.ResourceList{
							"memory": resource.MustParse("2Gi"),
						},
					},
					Env: []core.EnvVar{},
					SecurityContext: &core.SecurityContext{
						Privileged: &privileged,
					},
					LivenessProbe: &core.Probe{
						ProbeHandler: core.ProbeHandler{
							Exec: &core.ExecAction{
								Command: []string{
									"buildctl",
									"debug",
									"workers",
								},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       30,
					},
					VolumeMounts: []core.VolumeMount{{
						Name:      "buildkit-socket",
						MountPath: "/run/buildkit/",
					}},
				},
			},
			Volumes: []core.Volume{{
				Name: "buildkit-socket",
				VolumeSource: core.VolumeSource{
					EmptyDir: &core.EmptyDirVolumeSource{},
				},
			}},
		},
	}

	err := r.Client.Create(ctx, pod)

	return pod, err
}

func (r *BuildReconciler) tagFor(build *spot.Build) string {
	if build.Spec.Image.Tag == nil {
		return "latest"
	}

	return *build.Spec.Image.Tag
}

func (r *BuildReconciler) markBuildHasErrored(ctx context.Context, build *spot.Build, err error) error {
	r.EventRecorder.Event(build, "Warning", string(spot.BuildPhaseError), err.Error())
	build.Status.Phase = spot.BuildPhaseError
	return r.Client.Status().Update(ctx, build)
}
