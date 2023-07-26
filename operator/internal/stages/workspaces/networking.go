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

type Networking struct {
	client.Client
}

func (n *Networking) ingressRuleForNetwork(network *spot.ComponentNetworkSpec, workspace *spot.Workspace, serviceName string) (*networking.IngressRule, error) {
	rule := networking.IngressRule{
		Host: fmt.Sprintf("%s.%s.%s", network.Name, workspace.Spec.Tag, workspace.Spec.Host),
		IngressRuleValue: networking.IngressRuleValue{
			HTTP: &networking.HTTPIngressRuleValue{},
		},
	}

	path := networking.HTTPIngressPath{
		Path: network.Ingress.Path,
		Backend: networking.IngressBackend{
			Service: &networking.IngressServiceBackend{
				Name: serviceName,
				Port: networking.ServiceBackendPort{Number: int32(network.Port)},
			},
		},
	}

	if network.Ingress.PathType == nil {
		pathType := networking.PathTypePrefix
		path.PathType = &pathType
	} else {
		path.PathType = network.Ingress.PathType
	}

	rule.HTTP.Paths = append(rule.HTTP.Paths, path)

	return &rule, nil
}

func (n *Networking) Start(ctx context.Context, workspace *spot.Workspace) error {
	workspace.Status.Services = make(map[string]spot.ServiceReference)
	ingressClassName := "nginx"
	ingress := &networking.Ingress{
		ObjectMeta: meta.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", workspace.Name),
			Namespace:    workspace.Status.Namespace,
		},
		Spec: networking.IngressSpec{
			IngressClassName: &ingressClassName,
		},
	}

	for _, component := range workspace.Spec.Components {
		for _, network := range component.Networks {
			service := core.Service{
				ObjectMeta: meta.ObjectMeta{
					Name:      network.Name,
					Namespace: workspace.Status.Namespace,
				},
				Spec: core.ServiceSpec{
					Selector: map[string]string{
						"app.kubernetes.io/name": component.Name,
					},
					Ports: []core.ServicePort{
						{
							Port:       int32(network.Port),
							TargetPort: intstr.FromInt(network.Port),
						},
					},
				},
			}

			if err := n.Client.Create(ctx, &service); err != nil {
				return err
			}

			workspace.Status.Services[fmt.Sprintf("%s/%s", component.Name, network.Name)] = spot.NewServiceReference(&service)

			if network.Ingress != nil {
				ingressRule, err := n.ingressRuleForNetwork(&network, workspace, service.Name)
				if err != nil {
					return err
				}
				ingress.Spec.Rules = append(ingress.Spec.Rules, *ingressRule)
			}
		}
	}

	workspace.Status.Stage = spot.WorkspaceStageBuilding
	if err := n.Status().Update(ctx, workspace); err != nil {
		return err
	}

	return n.Client.Create(ctx, ingress)
}
