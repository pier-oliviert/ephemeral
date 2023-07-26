package v1alpha1

import (
	"errors"

	core "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
)

var ErrComponentEnvSourceFound = errors.New("could not find a value for the specified environment name")

type ComponentSpec struct {
	Name string `json:"name"`

	// Execute a different entrypoint command than the one
	// specified in the image
	Command []string `json:"command,omitempty"`

	// Links a component to an EnvironmentSpec entry.
	Environments []ComponentEnvironmentSpec `json:"environments,omitempty"`

	// Network service
	Networks []ComponentNetworkSpec `json:"networks,omitempty"`

	// Defines how the image is built for this component
	// The workspace will aggregate all the images at build time and
	// will deduplicate the images so only 1 unique image is built.
	Image ImageSpec `json:"image"`
}

func (c *ComponentSpec) GetEnvVars() []core.EnvVar {
	var envs []core.EnvVar

	for _, env := range c.Environments {
		envVar := core.EnvVar{Name: env.Name, Value: *env.Value}

		if len(env.Alias) != 0 {
			envVar.Name = env.Alias
		}

		envs = append(envs, envVar)
	}

	return envs
}

type ComponentEnvironmentSpec struct {
	// Name of the EnvironmentSpec at the Workspace level.
	// The name is going to be used as the name of the ENV inside
	// the component's pod.
	Name string `json:"name"`

	// If the Environment needs to have a different
	// name than the one specified, `as` can be used
	// to give it an alias.
	Alias string `json:"as,omitempty"`

	// Value generally  is going to be generated from the Workspace's `EnvironmentSpec`
	Value *string `json:"value,omitempty"`
}

type ComponentNetworkSpec struct {
	// If the Ingress field is set, an ingress will be created with the spec
	// +optional
	Ingress *ComponentIngressSpec `json:"ingress,omitempty"`

	// Needs to be unique within a component, will be used as a prefix for the Ingress's host
	// if the Ingress is set.
	Name string `json:"name"`

	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

type ComponentIngressSpec struct {
	// Path is matched agaisnt the path of the incoming request. Path must begin with
	// a '/'.
	// +kubebuilder:default:=/
	Path string `json:"path,omitempty"`

	// https://pkg.go.dev/k8s.io/api@v0.27.2/networking/v1#HTTPIngressPath
	// Defaults to Prefix
	// +optional
	PathType *networking.PathType `json:"path_type,omitempty"`
}
