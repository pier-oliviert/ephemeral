package workspaces

import (
	"context"
	"fmt"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AssignNamespace(ctx context.Context, workspace *spot.Workspace, c client.Client) error {
	if len(workspace.Status.Namespace) == 0 {
		namespace := core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				GenerateName: fmt.Sprintf("workspace-%s-", workspace.Spec.Tag),
			},
		}

		if err := c.Create(ctx, &namespace); err != nil {
			return err
		}

		workspace.Status.Namespace = namespace.Name
	}

	workspace.Status.Stage = spot.WorkspaceStageNetworking

	return c.Status().Update(ctx, workspace)
}
