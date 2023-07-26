package workspaces

import (
	"context"
	"fmt"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Deployment struct {
	client.Client
}

func (d *Deployment) Start(ctx context.Context, workspace *spot.Workspace) error {
	for _, component := range workspace.Spec.Components {
		envs, err := d.environmentsForComponent(&component, workspace)
		if err != nil {
			return err
		}
		imageName := component.Image.Name
		if component.Image.Tag != nil {
			imageName = fmt.Sprintf("%s:%s", imageName, *component.Image.Tag)
		}

		pod := core.Pod{
			ObjectMeta: meta.ObjectMeta{
				Name:      component.Name,
				Namespace: workspace.Status.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name": component.Name,
				},
			},
			Spec: core.PodSpec{
				RestartPolicy: core.RestartPolicyNever,
				Containers: []core.Container{
					{
						Name:            component.Name,
						Image:           imageName,
						ImagePullPolicy: core.PullAlways,
						Env:             envs,
					},
				},
			},
		}

		for _, network := range component.Networks {
			ref := workspace.Status.Services[fmt.Sprintf("%s/%s", component.Name, network.Name)]

			var service core.Service
			if err := d.Client.Get(ctx, ref.NamespacedName(), &service); err != nil {
				return err
			}

			container := pod.Spec.Containers[0]
			container.Ports = append(container.Ports, core.ContainerPort{
				Name:          service.Name,
				HostPort:      int32(service.Spec.Ports[0].Port),
				ContainerPort: int32(service.Spec.Ports[0].Port),
			},
			)
		}

		// TODO: Need to rework this when sidecar becomes a possibility.
		if len(component.Command) != 0 {
			pod.Spec.Containers[0].Command = component.Command
		}

		if err := d.Client.Create(ctx, &pod); err != nil {
			return err
		}
	}

	workspace.Status.Stage = spot.WorkspaceStageDeployed

	return d.Client.SubResource("status").Update(ctx, workspace)
}

func (d *Deployment) environmentsForComponent(component *spot.ComponentSpec, workspace *spot.Workspace) ([]core.EnvVar, error) {
	var environments []core.EnvVar

	for _, env := range component.Environments {
		envVar := core.EnvVar{}

		if len(env.Alias) != 0 {
			envVar.Name = env.Alias
		} else {
			envVar.Name = env.Name
		}

		if env.Value != nil {
			envVar.Value = *env.Value
		} else {
			value, err := d.valueForEnvironmentName(env.Name, workspace)

			if err != nil {
				// Most likely a user error, let's bail right now and
				// let the user correct his mistake.
				return nil, err
			}

			envVar.Value = value
		}

		environments = append(environments, envVar)
	}

	return environments, nil
}

func (d *Deployment) valueForEnvironmentName(name string, workspace *spot.Workspace) (string, error) {
	var value string

	for _, env := range workspace.Spec.Environments {
		if env.Name == name {
			value = env.Value
			break
		}
	}

	if len(value) == 0 {
		return value, fmt.Errorf("couldn't find an environment for %s", name)
	}

	return value, nil
}
