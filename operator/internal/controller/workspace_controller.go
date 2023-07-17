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

	"k8s.io/apimachinery/pkg/api/errors"
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

	// TODO: Temporary fix until the Admission hook is working
	// and the workspace defaults to Initialized.
	if workspace.Status.Stage == spot.WorkspaceStage("") {
		workspace.Status.Stage = spot.WorkspaceStageInitialized
		// Let's force the reconciler to run again instead. Not optimal, but as
		// mentioned above, this is temporary.
		if err := r.Status().Update(ctx, &workspace); err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
		}
		return ctrl.Result{}, nil
	}

	// TODO: Move this in admission webhook, this cannot stay here
	// because it causes a racing conditions as K8s doesn't support
	// updating spec & sub-resource in the same reconciler loop
	if !controllerutil.ContainsFinalizer(&workspace, stages.Finalizer) {
		controllerutil.AddFinalizer(&workspace, stages.Finalizer)
		if err := r.Client.Update(ctx, &workspace); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
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
			return ctrl.Result{}, err
		}
	}

	if !workspace.ObjectMeta.DeletionTimestamp.IsZero() {
		// Workspace is marked for deletion, need to clear the namespace
		if err := stages.DestroyNamespace(ctx, &workspace, r.Client); err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, err)
		}

	}

	return ctrl.Result{}, nil
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
