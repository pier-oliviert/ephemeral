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
limitations under the License.  */

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
	tasks "github.com/releasehub-com/spot/operator/internal/tasks/workspaces"
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

	// Ignore everything about this workspace if the workspace is scheduled to be deleted.
	if !workspace.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.terminate(ctx, &workspace)
	}

	switch workspace.Status.Phase {
	case "":
		workspace.Status.Phase = spot.WorkspacePhaseRunning
	case spot.WorkspacePhaseError:
		return ctrl.Result{}, nil
	}

	if condition := workspace.Status.Conditions.GetCondition(spot.WorkspaceConditionNamespace); condition.Status == spot.ConditionInitialized {
		err := tasks.AssignNamespace(ctx, &workspace, r.Client)
		if err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, &condition.Type, err)
		}
		r.EventRecorder.Event(&workspace, "Normal", "Initialized", fmt.Sprintf("Workspace initialized, assigned to namespace: %s", workspace.Status.Namespace))
	}

	// Can't move further until the namespace condition is properly setup
	if condition := workspace.Status.Conditions.GetCondition(spot.WorkspaceConditionNamespace); condition.Status != spot.ConditionSuccess {
		r.EventRecorder.Event(&workspace, "Normal", string(condition.Type), "Namespace not ready, waiting.")
		return ctrl.Result{}, nil
	}

	if condition := workspace.Status.Conditions.GetCondition(spot.WorkspaceConditionNetworking); condition.Status == spot.ConditionInitialized {
		r.EventRecorder.Event(&workspace, "Normal", "Networking", "Creating network resources for this workspace")
		networking := tasks.Networking{Client: r.Client}
		if err := networking.Start(ctx, &workspace); err != nil {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, &condition.Type, err)
		}
		return ctrl.Result{}, nil
	}

	if condition := workspace.Status.Conditions.GetCondition(spot.WorkspaceConditionImages); condition.Status != spot.ConditionSuccess {
		switch condition.Status {
		case spot.ConditionInitialized:
			r.EventRecorder.Event(&workspace, "Normal", string(spot.WorkspaceConditionImages), "deploying builders")
			if err := tasks.Build(ctx, &workspace, r.Client); err != nil {
				return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, &condition.Type, err)
			}
		case spot.ConditionError:
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, &condition.Type, fmt.Errorf("build failed"))
		default:
			if err := tasks.MonitorBuilds(ctx, &workspace, r.Client); err != nil {
				return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, &condition.Type, err)
			}
			r.EventRecorder.Event(&workspace, "Normal", string(spot.WorkspaceConditionImages), "waiting for builds to complete")
		}

		return ctrl.Result{}, nil
	}

	if workspace.Status.Conditions.GetCondition(spot.WorkspaceConditionImages).Status == spot.ConditionSuccess {
		condition := workspace.Status.Conditions.GetCondition(spot.WorkspaceConditionDeployment)
		if condition.Status == spot.ConditionInitialized {
			r.EventRecorder.Event(&workspace, "Normal", "Deploying", "Deploying services and updating routes")
			deployment := tasks.Deployment{Client: r.Client}
			if err := deployment.Start(ctx, &workspace); err != nil {
				return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, &workspace, &condition.Type, err)
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) terminate(ctx context.Context, workspace *spot.Workspace) (ctrl.Result, error) {
	namespace := core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Name: workspace.Status.Namespace,
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace); err != nil {
		// Termination can only deal with ErrNotFound. Anything else means
		// something unexpected happened.
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, workspace, nil, err)
		}

		if controllerutil.ContainsFinalizer(workspace, spot.WorkspaceFinalizer) {
			controllerutil.RemoveFinalizer(workspace, spot.WorkspaceFinalizer)
			if err := r.Update(ctx, workspace); err != nil {
				return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, workspace, nil, err)
			}

			return ctrl.Result{}, nil
		}
	}

	// If the naemspace is being deleted, let's wait for it to finish.
	if namespace.Status.Phase == core.NamespaceTerminating {
		return ctrl.Result{Requeue: true}, nil
	}

	r.EventRecorder.Event(workspace, "Normal", string(spot.WorkspacePhaseTerminating), "Removing all associated resources with workspace")

	workspace.Status.Phase = spot.WorkspacePhaseTerminating
	if err := r.Status().Update(ctx, workspace); err != nil {
		return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, workspace, nil, err)
	}

	if err := r.Delete(ctx, &namespace); err != nil {
		return ctrl.Result{}, r.markWorkspaceHasErrored(ctx, workspace, nil, err)
	}

	return ctrl.Result{Requeue: true}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&spot.Workspace{}).
		Owns(&spot.Workspace{}).
		Complete(r)
}

func (r *WorkspaceReconciler) markWorkspaceHasErrored(ctx context.Context, workspace *spot.Workspace, conditionType *spot.WorkspaceConditionType, err error) error {
	r.EventRecorder.Event(workspace, "Warning", string(spot.WorkspaceStageError), err.Error())
	if conditionType != nil {
		workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
			Type:   *conditionType,
			Status: spot.ConditionError,
		})
	}
	workspace.Status.Phase = spot.WorkspacePhaseError
	return r.Client.Status().Update(ctx, workspace)
}
