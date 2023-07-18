package workspaces

import (
	"context"
	"fmt"

	core "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Deployment struct {
	client.Client
}

func (d *Deployment) Start(ctx context.Context, workspace *spot.Workspace) error {
	services := make(map[string]*core.Service)

	for _, component := range workspace.Spec.Components {
		service := core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name:      component.Name,
				Namespace: workspace.Status.Namespace,
			},
			Spec: core.ServiceSpec{
				Selector: map[string]string{
					"app.kubernetes.io/name": component.Name,
				},
				Ports: []core.ServicePort{
					{
						Name:       component.Name,
						Port:       int32(component.Services[0].Port),
						TargetPort: intstr.FromInt(component.Services[0].Port),
					},
				},
			},
		}

		if err := d.Client.Create(ctx, &service); err != nil {
			return err
		}

		services[component.Name] = &service

		// TODO: move domain as a configuration field for a project/workspace
		domain := "po.ngrok.app"

		for _, serviceSpec := range component.Services {
			if len(serviceSpec.Ingress) == 0 {
				continue
			}

			ingressClassName := "nginx"
			pathType := networking.PathTypePrefix

			ingress := &networking.Ingress{
				ObjectMeta: meta.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-", component.Name),
					Namespace:    workspace.Status.Namespace,
				},
				Spec: networking.IngressSpec{
					IngressClassName: &ingressClassName,
					Rules: []networking.IngressRule{{
						Host: fmt.Sprintf("%s.%s", serviceSpec.Ingress, domain),
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{{
									Path:     "/",
									PathType: &pathType,
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: component.Name,
											Port: networking.ServiceBackendPort{Number: services[component.Name].Spec.Ports[0].Port},
										},
									},
								}},
							},
						},
					}},
				},
			}

			if err := d.Client.Create(ctx, ingress); err != nil {
				return err
			}
		}
	}

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
						Name:  component.Name,
						Image: imageName,
						Ports: []core.ContainerPort{
							{
								Name:          component.Services[0].Protocol,
								HostPort:      int32(component.Services[0].Port),
								ContainerPort: int32(component.Services[0].Port),
							},
						},
						Env: envs,
					},
				},
			},
		}

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
