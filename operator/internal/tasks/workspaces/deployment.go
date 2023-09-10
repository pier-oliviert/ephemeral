package workspaces

import (
	"context"
	"fmt"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Deployer struct {
	client.Client
	record.EventRecorder
}

// Reconcile is a sub-reconcile loop that will manage the reconcilation process for the `spot.WorkspaceconditionDeployment`.
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
func (d *Deployer) Reconcile(ctx context.Context, workspace *spot.Workspace, condition *spot.WorkspaceCondition) (ctrl.Result, error) {
	d.EventRecorder.Event(workspace, "Normal", "Deploying", "Deploying services and updating routes")

	for _, component := range workspace.Spec.Components {
		envs, err := d.environmentsForComponent(&component, workspace)
		if err != nil {
			return ctrl.Result{}, err
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
				return ctrl.Result{}, err
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
			return ctrl.Result{}, err
		}
	}

	workspace.Status.Stage = spot.WorkspaceStageDeployed

	return ctrl.Result{}, d.Client.SubResource("status").Update(ctx, workspace)
}

func (d *Deployer) environmentsForComponent(component *spot.ComponentSpec, workspace *spot.Workspace) ([]core.EnvVar, error) {
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

func (d *Deployer) valueForEnvironmentName(name string, workspace *spot.Workspace) (string, error) {
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
