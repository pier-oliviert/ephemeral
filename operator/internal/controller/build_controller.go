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
	"time"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
)

var ErrStageWithInvalidState = errors.New("stage did not match the status of the build")

// BuildReconciler reconciles a Build object
type BuildReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	record.EventRecorder
}

//+kubebuilder:rbac:groups=spot.release.com,resources=builds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=spot.release.com,resources=builds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=spot.release.com,resources=builds/finalizers,verbs=update

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

	switch build.Status.Stage {
	// BuildStageInitialized is the default stage for a Build which means
	// the Pod that builds the image has not been dispatched yet.
	// The goal here is to create a Pod and update the Build stage to BuildStageRunning
	// so the reconciler can move forward.
	case spot.BuildStageInitialized:
		if build.Status.Pod != nil {
			// We're not supposed to have a Pod associated to this build yet,
			// this is an unrecoverable error.
			return ctrl.Result{Requeue: false}, r.markBuildHasErrored(ctx, &build, ErrStageWithInvalidState)
		}

		pod, err := r.buildPod(ctx, &build)
		if err != nil {
			logger.Info("Oops", "error", err)
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}

		podReference := spot.NewPodReference(pod)
		build.Status.Pod = &podReference
		build.Status.Stage = spot.BuildStageRunning
		if err := r.Client.Status().Update(ctx, &build); err != nil {
			logger.Info("Oops", "error", err)
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}

	case spot.BuildStageDone:
		// Let's update the status on the Workspace now that a build for that workspace is done.
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

			// Error is not of type not found, can't recover from this
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}

		r.EventRecorder.Event(&build, "Normal", string(build.Status.Stage), fmt.Sprintf("Clearing the builder pod(%s/%s)", pod.Namespace, pod.Name))

		if err := r.Client.Delete(ctx, &pod); err != nil {
			r.EventRecorder.Event(&build, "Warning", string(build.Status.Stage), fmt.Sprintf("Could not delete the pod as part of housekeeping, pod: %s/%s", pod.Namespace, pod.Name))
		}

	case spot.BuildStageError:
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
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, ErrStageWithInvalidState)
		}

		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: build.Namespace, Name: reference.Name}, &workspace); err != nil {
			return ctrl.Result{Requeue: false}, r.markBuildHasErrored(ctx, &build, err)
		}

		// TODO: Workspace CRD should watch for builds and should update
		// its own stage.
		workspace.Status.Stage = spot.WorkspaceStageError
		if err := r.Client.SubResource("status").Update(ctx, &workspace); err != nil {
			logger.Error(err, "fatal error updating the workspace status")
		}

	default:
		var pod core.Pod
		if err := r.Client.Get(ctx, build.Status.Pod.NamespacedName(), &pod); err != nil {
			// Pod doesn't exist but we're waiting on it to update us. It's an unrecoverable error here
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, err)
		}

		if pod.Status.Phase == core.PodFailed {
			// Pod should be running as this reconciler is waiting for an update from it.
			// It's an unrecoverable error.
			return ctrl.Result{}, r.markBuildHasErrored(ctx, &build, fmt.Errorf("builder pod was unexpectly terminated: %s", pod.Status.Phase))
		}

		// The Build is self managed at this point in time, let's wait until we can act
		// on the BuildStage
		r.EventRecorder.Event(&build, "Normal", string(build.Status.Stage), fmt.Sprintf("Waiting on build information from the builder pod %s", build.Status.Pod.String()))

		// Since the builder wants to watch for a specific pod, it might be better here to
		// poll the pod every 5 seconds than creating a watcher for the pods in the manager.
		// I'm not entirely sure and there might be another way to watch a single pod for a single resource
		// that I'm not aware of at the moment.
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&spot.Build{}).
		Complete(r)
}

func (r *BuildReconciler) buildPod(ctx context.Context, build *spot.Build) (*core.Pod, error) {
	privileged := true
	hostPathType := core.HostPathDirectoryOrCreate
	pod := &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Namespace:    build.Namespace,
			GenerateName: fmt.Sprintf("%s-", build.Name),
			Annotations: map[string]string{
				"container.apparmor.security.beta.kubernetes.io/buildkit": "unconfined",
				"container.seccomp.security.alpha.kubernetes.io/buildkit": "unconfined",
			},
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
				Image:           "builder:latest", // TODO: Need to replace this with real image
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
						Value: build.Spec.RepositoryURL,
					},
					{
						Name:  "IMAGE_URL",
						Value: build.ImageURL(),
					},
					{
						Name:  "IMAGE_TAG",
						Value: r.tagFor(build),
					},
					{
						Name: "REGISTRY_AUTH",
						ValueFrom: &core.EnvVarSource{
							ConfigMapKeyRef: &core.ConfigMapKeySelector{
								LocalObjectReference: core.LocalObjectReference{
									Name: "click-mania",
								},
								Key: "registry.auth",
							},
						},
					},
				},
				SecurityContext: &core.SecurityContext{
					Privileged: &privileged,
				},
				VolumeMounts: []core.VolumeMount{
					{
						Name:      "cache",
						MountPath: "/var/lib/buildkit",
					},
				},
			}},
			Volumes: []core.Volume{{
				Name: "cache",
				VolumeSource: core.VolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/var/lib/buildkit",
						Type: &hostPathType,
					},
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
	logger := log.FromContext(ctx)
	logger.Error(err, "Error happened with the build")
	build.Status.Stage = spot.BuildStageError
	return r.Client.Status().Update(ctx, build)
}
