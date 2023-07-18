package v1alpha1

import (
	"errors"

	core "k8s.io/api/core/v1"
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
	Services []ServiceSpec `json:"services"`

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
