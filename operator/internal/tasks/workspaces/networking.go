package workspaces

import (
	"context"
	"fmt"

	core "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Networking struct {
	client.Client
	record.EventRecorder
}

// Name of the cert-manager ClusterIssuer CRD that will be used to generate the
// TLS Certificate for each of the Ingress created for workspaces.
// This issuer is *required* as the deployment will not
// include a TLS certificate if this resource does not exist in the cluster.
// This might become a configurable field in the future.
const CertClusterIssuerName = "spot-workspace-issuer"

// Reconcile is a sub-reconcile loop that will manage the reconcilation process for the `spot.WorkspaceconditionNetworking`.
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
func (n *Networking) Reconcile(ctx context.Context, workspace *spot.Workspace, condition *spot.WorkspaceCondition) (ctrl.Result, error) {
	workspace.Status.Services = make(map[string]spot.Reference)
	ingressClassName := "nginx"
	ingress := &networking.Ingress{
		ObjectMeta: meta.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", workspace.Name),
			Namespace:    workspace.Status.Namespace,
			Annotations: map[string]string{
				"cert-manager.io/cluster-issuer":              CertClusterIssuerName,
				"cert-manager.io/issue-temporary-certificate": "true",
				"acme.cert-manager.io/http01-edit-in-place":   "true",
			},
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
				return ctrl.Result{}, err
			}

			workspace.Status.Services[fmt.Sprintf("%s/%s", component.Name, network.Name)] = *spot.NewReference(&service)

			if network.Ingress != nil {
				ingressRule, err := n.ingressRuleForNetwork(&network, workspace, service.Name)
				if err != nil {
					return ctrl.Result{}, err
				}
				ingress.Spec.Rules = append(ingress.Spec.Rules, *ingressRule)
			}
		}
	}

	var hosts []string

	for _, rule := range ingress.Spec.Rules {
		hosts = append(hosts, rule.Host)
	}

	ingress.Spec.TLS = []networking.IngressTLS{{
		Hosts:      hosts,
		SecretName: fmt.Sprintf("%s-ingress-cert", workspace.Name),
	}}

	workspace.Status.Conditions.SetCondition(&spot.WorkspaceCondition{
		Type:   spot.WorkspaceConditionNetworking,
		Status: spot.ConditionSuccess,
	})

	if err := n.Status().Update(ctx, workspace); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, n.Client.Create(ctx, ingress)
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
