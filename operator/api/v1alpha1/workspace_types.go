/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const WorkspaceFinalizer = "spot.release.com/namespace"

type WorkspaceStage string

// +kubebuilder:validation:Enum=Initialized;Networking;Building;Deploying;Deployed;Updating;Errored;Terminating
const (
	WorkspaceStageInitialized WorkspaceStage = ""
	WorkspaceStageNetworking  WorkspaceStage = "Networking"
	WorkspaceStageBuilding    WorkspaceStage = "Building"
	WorkspaceStageDeploying   WorkspaceStage = "Deploying"
	WorkspaceStageDeployed    WorkspaceStage = "Deployed"
	WorkspaceStageUpdating    WorkspaceStage = "Updating"
	WorkspaceStageError       WorkspaceStage = "Errored"
	WorkspaceStageTerminating WorkspaceStage = "Terminating"
)

type WorkspaceSpec struct {
	// The host that components can use to generate ingresses.
	// This list assumes that there is a load balancer that can
	// accept any of these host upstream and can direct them to
	// the ingress controller.
	//
	// The domains here will be prefixed by the Workspace tag and the components'
	// network name.
	//
	// # Example
	//
	//	tag: "my-workspace"
	// 	host: release.com
	// 	components:
	// 	- name: backend
	// 		network:
	// 		  name: app
	//
	// For the `backend` component, if an ingress is created, it would be configured
	// to listen to `app.my-workspace.release.com`
	Host string `json:"host"`

	// Collection of all the components that are required for this
	// workspace to deploy.
	Components []ComponentSpec `json:"components,omitempty"`

	// Defines all the environments that will be needed for this workspace
	Environments []EnvironmentSpec `json:"environments"`

	// Default tag for all the images that are build that don't
	// have a tag specified to them. If no value is set,
	// it will be created before the builds starts.
	// A tag needs to be a valid DNS_LABEL and as such, it needs to
	// start with an alphabetic character (no numbers)
	// +optional
	Tag string `json:"tag,omitempty"`
}

type EnvironmentSpec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WorkspaceStatus defines the observed state of Workspace
type WorkspaceStatus struct {
	// ManagedNamespace is the namespace that will be associated with this workspace.
	// All k8s objects that will need to exist for this workspace will live under that
	// namespace
	Namespace string `json:"namespace,omitempty"` //omitempty until the code exists

	Stage WorkspaceStage `json:"stage,omitempty"`

	// Builds are the unit of work associated for each of the builds
	// that are required for this workspace to launch. Builds are seeding
	// the Images as they complete.
	Builds []BuildReference `json:"builds,omitempty"`

	// Images are seeded by Builds as they are completed. It's
	// also possible for some services in a workspace to have images that don't
	// require a build (think database, etc.).
	Images map[string]BuildImage `json:"images,omitempty"`

	// References to services that are created for this workspace.
	// These service are needed to figure out ports mapping for the
	// container when the workspace is in the Deploying stage.
	Services map[string]ServiceReference `json:"services,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.status.stage`

// Workspace is the Schema for the workspaces API
type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceSpec   `json:"spec,omitempty"`
	Status WorkspaceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
