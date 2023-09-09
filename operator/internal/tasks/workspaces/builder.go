package workspaces

import (
	"context"
	"errors"
	"fmt"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Builder struct {
	// UnrecoverableErrCallback is provided by the main reconcile loop to be called when an unrecoverable error
	// happens during the builder sub-reconcile loop and the workspace needs to set itself to "failed".
	UnrecoverableErrCallback func(context.Context, *spot.Workspace, *spot.WorkspaceConditionType, error) error

	client.Client
	record.EventRecorder
}

// Reconcile is a sub-reconcile loop that will manage the reconcilation process for the `spot.WorkspaceconditionBuildingImages`.
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
func (b *Builder) Reconcile(ctx context.Context, workspace *spot.Workspace, condition *spot.WorkspaceCondition) (ctrl.Result, error) {
	// The Builder condition is initialized which means it's ready to build all the image for this workspace.
	if condition.Status == spot.ConditionInitialized {
		if err := b.Build(ctx, workspace); err != nil {
			return ctrl.Result{}, b.UnrecoverableErrCallback(ctx, workspace, &condition.Type, err)
		}
	}

	// At this point, a build has been dispatched for each of the components that needs to be built.
	// Looking at the build status to see if there's something that needs to be done about them.
	// At this point, only two states are of interests: Error & Done.
	// Done means the workspace can move to the next sub-reconcile loop, a failure would mean
	// the workspace needs to be marked as failed for the user to be notified of the error.
	for _, ref := range workspace.Status.Builds {
		var build spot.Build
		if err := b.Client.Get(ctx, ref.NamespacedName(), &build); err != nil {
			return ctrl.Result{}, b.UnrecoverableErrCallback(ctx, workspace, &condition.Type, err)
		}

		switch build.Status.Phase {
		case spot.BuildPhaseError:
			return ctrl.Result{}, b.UnrecoverableErrCallback(ctx, workspace, &condition.Type, fmt.Errorf("build failed"))

		case spot.BuildPhaseDone:
			workspace.Status.Images = append(workspace.Status.Images, *build.Status.Image)
		}

	}

	// If the workspace's images size is not equal to the length
	// of build references, it means that there are still builds that
	// needs to be completed and the sub-reconciler needs to run again in
	// the future.
	if len(workspace.Status.Images) != len(workspace.Status.Builds) {
		return ctrl.Result{}, nil
	}

	workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
		Type:   spot.WorkspaceConditionBuildingImages,
		Status: spot.ConditionSuccess,
	})

	if err := b.Status().Update(ctx, workspace); err != nil {
		return ctrl.Result{}, b.UnrecoverableErrCallback(ctx, workspace, &condition.Type, err)
	}

	return ctrl.Result{}, nil
}

func (b *Builder) Build(ctx context.Context, workspace *spot.Workspace) error {
	logger := log.FromContext(ctx)
	b.EventRecorder.Event(workspace, "Normal", string(spot.WorkspaceConditionBuildingImages), "deploying builders")

	if len(workspace.Status.Builds) != 0 {
		return errors.New("unexpected builds present for this workspace")
	}

	var builds []*spot.Build
	for _, component := range workspace.Spec.Components {
		if component.Image.Repository == nil {
			// This image is not going to be built, let's exclude it from the build slice
			continue
		}

		build := &spot.Build{
			ObjectMeta: meta.ObjectMeta{
				Namespace:    workspace.Namespace,
				GenerateName: fmt.Sprintf("%s-", component.Name),
				OwnerReferences: []meta.OwnerReference{
					{
						Kind:       workspace.Kind,
						Name:       workspace.Name,
						APIVersion: workspace.APIVersion,
						UID:        workspace.UID,
					},
				},
			},
			Spec: spot.BuildSpec{
				Image: component.Image,
			},
		}

		builds = append(builds, build)
	}

	if len(builds) == 0 {
		workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
			Type:   spot.WorkspaceConditionBuildingImages,
			Status: spot.ConditionSuccess,
		})
		return b.Status().Update(ctx, workspace)
	}

	var references []spot.Reference
	for _, build := range builds {
		if err := b.Create(ctx, build); err != nil {
			logger.Error(err, "unexpected error creating a build")
			return err
		}

		references = append(references, build.GetReference())
	}

	workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
		Type:   spot.WorkspaceConditionBuildingImages,
		Status: spot.ConditionInProgress,
	})
	workspace.Status.Builds = references

	return b.Status().Update(ctx, workspace)
}
