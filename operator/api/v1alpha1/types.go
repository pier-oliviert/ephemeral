package v1alpha1

import (
	"fmt"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type PodReference struct {
	// `namespace` is the namespace of the pod.
	// Required
	Namespace string `json:"namespace" protobuf:"bytes,1,opt,name=namespace"`
	// `name` is the name of the pod.
	// Required
	Name string `json:"name" protobuf:"bytes,2,opt,name=name"`
}

func NewPodReference(pod *core.Pod) *PodReference {
	return &PodReference{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}
}

func (p PodReference) String() string {
	return fmt.Sprintf("%s/%s", p.Namespace, p.Name)
}

func (p PodReference) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      p.Name,
		Namespace: p.Namespace,
	}
}

type ServiceReference struct {
	// `namespace` is the namespace of the build.
	// Required
	Namespace string `json:"namespace" protobuf:"bytes,1,opt,name=namespace"`
	// `name` is the name of the build.
	// Required
	Name string `json:"name" protobuf:"bytes,2,opt,name=name"`
}

func NewServiceReference(service *core.Service) ServiceReference {
	return ServiceReference{
		Namespace: service.Namespace,
		Name:      service.Name,
	}
}

func (s ServiceReference) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      s.Name,
		Namespace: s.Namespace,
	}
}
func (s ServiceReference) String() string {
	return fmt.Sprintf("%s/%s", s.Namespace, s.Name)
}

type BuildReference struct {
	// `namespace` is the namespace of the build.
	// Required
	Namespace string `json:"namespace" protobuf:"bytes,1,opt,name=namespace"`
	// `name` is the name of the build.
	// Required
	Name string `json:"name" protobuf:"bytes,2,opt,name=name"`
}

func (b BuildReference) String() string {
	return fmt.Sprintf("%s/%s", b.Namespace, b.Name)
}
