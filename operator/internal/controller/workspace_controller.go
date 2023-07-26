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
	"fmt"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	stages "github.com/releasehub-com/spot/operator/internal/stages/workspaces"
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	record.EventRecorder
}

//+kubebuilder:rbac:groups=spot.release.com,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=spot.release.com,resources=workspaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=spot.release.com,resources=workspaces/finalizers,verbs=update
//+kubebuilder:rbac:groups=spot.release.com,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=namespaces;services,verbs=get;watch;list;create;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;watch;list;create;delete

func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var workspace spot.Workspace
	if err := r.Client.Get(ctx, req.NamespacedName, &workspace); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Couldn't retrieve the workspace", "NamespacedName", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	if !workspace.ObjectMeta.DeletionTimestamp.IsZero() {
		// Set the stage to Terminating so the following switch case
		// can properly terminate the workspace
		workspace.Status.Stage = spot.WorkspaceStageTerminating
		if err := r.Status().Update(ctx, &workspace); err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
		}
	}

	switch workspace.Status.Stage {

	// The Workspace was just created and nothing has happened to it
	// yet. The first step is to start the building process.
	case spot.WorkspaceStageInitialized:
		// Let's first create a namespace for the workspace
		err := stages.AssignNamespace(ctx, &workspace, r.Client)
		if err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
		}

		r.EventRecorder.Event(&workspace, "Normal", "Initialized", fmt.Sprintf("Workspace initialized, assigned to namespace: %s", workspace.Status.Namespace))

	case spot.WorkspaceStageNetworking:
		r.EventRecorder.Event(&workspace, "Normal", "Networking", "Creating network resources for this workspace")
		networking := stages.Networking{Client: r.Client}
		if err := networking.Start(ctx, &workspace); err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
		}
	// The Workspace launched the builders but those have not
	// completed yet. Need to monitor each of the builder object
	// to see if they are completed and we can move forward to the next
	// stage
	case spot.WorkspaceStageBuilding:
		if len(workspace.Status.Builds) == 0 {
			r.EventRecorder.Event(&workspace, "Normal", string(spot.WorkspaceStageBuilding), "deploying builders")
			if err := stages.Build(ctx, &workspace, r.Client); err != nil {
				return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
			}
		} else {
			if err := stages.MonitorBuilds(ctx, &workspace, r.Client); err != nil {
				return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
			}

			if workspace.Status.Stage == spot.WorkspaceStageBuilding {
				r.EventRecorder.Event(&workspace, "Normal", string(spot.WorkspaceStageBuilding), "waiting for builds to complete")
			}
		}

	case spot.WorkspaceStageDeploying:
		r.EventRecorder.Event(&workspace, "Normal", "Deploying", "Deploying services and updating routes")
		deployment := stages.Deployment{Client: r.Client}
		if err := deployment.Start(ctx, &workspace); err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
		}

	case spot.WorkspaceStageTerminating:
		r.EventRecorder.Event(&workspace, "Normal", string(spot.WorkspaceStageTerminating), "Removing all associated resources with workspace")
		if err := r.terminate(ctx, &workspace); err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
		}

		// Needs to re-enqueue until the finalizer is removed, otherwise, the workspace
		// might not be GC until the delay is reached
		if controllerutil.ContainsFinalizer(&workspace, spot.WorkspaceFinalizer) {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) terminate(ctx context.Context, workspace *spot.Workspace) error {
	logger := log.FromContext(ctx)
	namespace := core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Name: workspace.Status.Namespace,
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace); err != nil {
		logger.Info("Error finding the namespace")
		if errors.IsNotFound(err) {
			logger.Info("Removing the finalizer")
			controllerutil.RemoveFinalizer(workspace, spot.WorkspaceFinalizer)
			return r.Update(ctx, workspace)
		}

		// Only knows how to recover from a NotFound, everything
		// else is unrecoverable.
		return err
	}

	if namespace.Status.Phase == core.NamespaceTerminating {
		// Nothing more to do with this namespace, it will clear itself,
		// the finalizer can be removed from the workspace
		controllerutil.RemoveFinalizer(workspace, spot.WorkspaceFinalizer)
		return r.Update(ctx, workspace)
	}

	return r.Delete(ctx, &namespace)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&spot.Workspace{}).
		Complete(r)
}

func (r *WorkspaceReconciler) markWorkspaceHasErrored(ctx context.Context, workspace *spot.Workspace, err error) error {
	r.EventRecorder.Event(workspace, "Warning", string(spot.WorkspaceStageError), err.Error())
	workspace.Status.Stage = spot.WorkspaceStageError
	return r.Client.Status().Update(ctx, workspace)
}
