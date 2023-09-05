package workspaces

import (
	"context"
	"errors"
	"fmt"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Build(ctx context.Context, workspace *spot.Workspace, c client.Client) error {
	logger := log.FromContext(ctx)

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
			Type:   spot.WorkspaceConditionImages,
			Status: spot.ConditionSuccess,
		})
		return c.Status().Update(ctx, workspace)
	}

	var references []spot.Reference
	for _, build := range builds {
		if err := c.Create(ctx, build); err != nil {
			logger.Error(err, "unexpected error creating a build")
			return err
		}

		references = append(references, build.GetReference())
	}

	workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
		Type:   spot.WorkspaceConditionImages,
		Status: spot.ConditionInProgress,
	})
	workspace.Status.Builds = references

	return c.Status().Update(ctx, workspace)
}

func MonitorBuilds(ctx context.Context, workspace *spot.Workspace, c client.Client) error {
	buildsForWorkspace := 0

	for _, component := range workspace.Spec.Components {
		if component.Image.Registry != nil {
			buildsForWorkspace += 1
		}
	}

	if len(workspace.Status.Images) != buildsForWorkspace {
		return nil
	}

	workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
		Type:   spot.WorkspaceConditionImages,
		Status: spot.ConditionSuccess,
	})

	return c.Status().Update(ctx, workspace)
}
