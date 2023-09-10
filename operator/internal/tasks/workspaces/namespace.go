package workspaces

import (
	"context"
	"fmt"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Namespacer struct {
	client.Client
	record.EventRecorder
}

// Reconcile is a sub-reconcile loop that will manage the reconcilation process for the `spot.WorkspaceconditionNamespace`.
// This function returns the value that the main reconcile loop should use to terminate the current reconciliation for this
// custom resource. It's an error to try to run this sub reconcile loop with any other as that will break the fundamental rules
// of reconciliation.
//
// The sub-reconciliation loops are built in this package to help readers reason about the flow and it has no technical
// purposes.
//
// Reconcile takes the workspace it's operating on as well as the condition. The condition could be implied here but since
// it's already retrieved it to reach this state (the main reconciliation loop need to lookup the condition before calling this
// sub-reconcile loop), it makes sense to just pass it here.
func (n *Namespacer) Reconcile(ctx context.Context, workspace *spot.Workspace, condition *spot.WorkspaceCondition) (ctrl.Result, error) {
	if len(workspace.Status.Namespace) == 0 {
		namespace := core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				GenerateName: fmt.Sprintf("workspace-%s-", workspace.Spec.Tag),
			},
		}

		if err := n.Client.Create(ctx, &namespace); err != nil {
			return ctrl.Result{}, err
		}

		workspace.Status.Namespace = namespace.Name
	}

	workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
		Type:   spot.WorkspaceConditionNamespace,
		Status: spot.ConditionSuccess,
	})

	if err := n.Client.Status().Update(ctx, workspace); err != nil {
		return ctrl.Result{}, err
	}

	n.EventRecorder.Event(workspace, "Normal", "Initialized", fmt.Sprintf("Workspace initialized, assigned to namespace: %s", workspace.Status.Namespace))

	return ctrl.Result{}, nil
}
