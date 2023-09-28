package builds

import (
	"context"
	"fmt"
	"strings"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/env"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodDeployment struct {
	client.Client
	record.EventRecorder
}

func (p *PodDeployment) Reconcile(ctx context.Context, build *spot.Build, condition *spot.BuildCondition) (ctrl.Result, error) {
	var secret core.Secret
	if err := p.Client.Get(ctx, client.ObjectKey{Name: build.Spec.SecretRef, Namespace: build.Namespace}, &secret); err != nil {
		return ctrl.Result{}, err
	}

	// The Build is just initialized and nothing has been processed, yet. For the Build to actually start, a pod
	// needs to be scheduled with the right service account so that it can update the state of the Build has it goes
	// through each of the steps.
	pod := p.pod(build, &secret)
	err := p.Client.Create(ctx, pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	// It's important to set the condition first before calling conditions.Phase() as otherwise it would
	// not include the state of this condition when deriving the value.
	build.Status.SetCondition(spot.BuildCondition{
		Type:   spot.BuildConditionDeployPod,
		Status: spot.ConditionInProgress,
	})

	build.Status.Pod = spot.NewReference(pod)
	build.Status.Phase = build.Status.Conditions.Phase()

	if err := p.Client.Status().Update(ctx, build); err != nil {
		return ctrl.Result{}, err
	}

	build.Status.SetCondition(spot.BuildCondition{
		Type:   spot.BuildConditionDeployPod,
		Status: spot.ConditionInProgress,
	})

	build.Status.Pod = spot.NewReference(pod)
	build.Status.Phase = build.Status.Conditions.Phase()

	if err := p.Client.Status().Update(ctx, build); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (p *PodDeployment) pod(build *spot.Build, secret *core.Secret) *core.Pod {
	privileged := true
	return &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Namespace:    build.Namespace,
			GenerateName: fmt.Sprintf("build-%s-", build.Name),
			Annotations:  map[string]string{},
			OwnerReferences: []meta.OwnerReference{
				{
					APIVersion: build.APIVersion,
					Kind:       build.Kind,
					Name:       build.Name,
					UID:        build.UID,
				},
			},
		},
		Spec: core.PodSpec{
			RestartPolicy:      core.RestartPolicyNever,
			ServiceAccountName: "spot-controller-manager", // TODO: Most likely to change spot-system/default to support the RBAC settings we need instead
			Containers: []core.Container{{
				Name:            "buildkit",
				Image:           env.GetString("BUILDER_IMAGE", "builder:dev"),
				ImagePullPolicy: core.PullNever,
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
					Limits: core.ResourceList{
						"memory": resource.MustParse("2Gi"),
					},
				},
				Env: []core.EnvVar{
					{
						Name:  "BUILD_REFERENCE",
						Value: build.GetReference().String(),
					},
					{
						Name:  "REPOSITORY_URL",
						Value: build.Spec.Image.Repository.URL,
					},
					{
						Name:  "REPOSITORY_REF",
						Value: build.Spec.Image.Repository.Ref,
					},
					{
						Name:  "IMAGE_URL",
						Value: build.Spec.Image.Registry.URL,
					},
					{
						Name:  "IMAGE_TAGS",
						Value: strings.Join(build.Spec.Image.Registry.Tags, ","),
					},
					{
						Name: "REPOSITORY_SECRETS",
						ValueFrom: &core.EnvVarSource{
							SecretKeyRef: &core.SecretKeySelector{
								LocalObjectReference: core.LocalObjectReference{
									Name: secret.Name,
								},
								Key: spot.BuildSecretRepositories,
							},
						},
					},
					{
						Name: "REGISTRY_SECRETS",
						ValueFrom: &core.EnvVarSource{
							SecretKeyRef: &core.SecretKeySelector{
								LocalObjectReference: core.LocalObjectReference{
									Name: secret.Name,
								},
								Key: spot.BuildSecretRegistries,
							},
						},
					},
				},
				SecurityContext: &core.SecurityContext{
					Privileged: &privileged,
				},
				VolumeMounts: []core.VolumeMount{{
					Name:      "buildkit-socket",
					MountPath: "/run/buildkit/",
				},
				},
			},
				{
					Name:  "buildkitd",
					Image: "moby/buildkit:master",
					Resources: core.ResourceRequirements{
						Requests: core.ResourceList{
							"memory": resource.MustParse("1Gi"),
						},
						Limits: core.ResourceList{
							"memory": resource.MustParse("2Gi"),
						},
					},
					Env: []core.EnvVar{},
					SecurityContext: &core.SecurityContext{
						Privileged: &privileged,
					},
					LivenessProbe: &core.Probe{
						ProbeHandler: core.ProbeHandler{
							Exec: &core.ExecAction{
								Command: []string{
									"buildctl",
									"debug",
									"workers",
								},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       30,
					},
					VolumeMounts: []core.VolumeMount{{
						Name:      "buildkit-socket",
						MountPath: "/run/buildkit/",
					}},
				},
			},
			Volumes: []core.Volume{{
				Name: "buildkit-socket",
				VolumeSource: core.VolumeSource{
					EmptyDir: &core.EmptyDirVolumeSource{},
				},
			}},
		},
	}
}
