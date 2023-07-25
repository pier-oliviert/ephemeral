package workspaces

import (
	"context"
	"fmt"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const Finalizer = "spot.release.com/namespace"

func AssignNamespace(ctx context.Context, workspace *spot.Workspace, c client.Client) error {
	if len(workspace.Status.Namespace) == 0 {
		namespace := core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				GenerateName: fmt.Sprintf("workspace-%s", workspace.Spec.Tag),
			},
		}

		if err := c.Create(ctx, &namespace); err != nil {
			return err
		}

		workspace.Status.Namespace = namespace.Name
	}

	workspace.Status.Stage = spot.WorkspaceStageBuilding

	return c.Status().Update(ctx, workspace)
}

func DestroyNamespace(ctx context.Context, workspace *spot.Workspace, c client.Client) error {
	if controllerutil.ContainsFinalizer(workspace, Finalizer) {
		if len(workspace.Status.Namespace) != 0 {
			namespace := core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name: workspace.Status.Namespace,
				},
			}

			if err := c.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace); err != nil {
				return err
			}

			if err := c.Delete(ctx, &namespace); err != nil {
				return err
			}
		}

		controllerutil.RemoveFinalizer(workspace, Finalizer)
		if err := c.Update(ctx, workspace); err != nil {
			return err
		}
	}

	return nil
}
