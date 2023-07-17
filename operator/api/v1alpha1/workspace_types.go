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

// +kubebuilder:validation:Enum=Building;Deploying;Running;Updating;Errored;Terminating;Deleted
type WorkspaceStage string

const (
	WorkspaceStageInitialized WorkspaceStage = "" // TODO: I would prefer if it was a string, but I can't get the enum to default to this otherwise.
	WorkspaceStageBuilding    WorkspaceStage = "Building"
	WorkspaceStageDeploying   WorkspaceStage = "Deploying"
	WorkspaceStageRunning     WorkspaceStage = "Running"
	WorkspaceStageUpdating    WorkspaceStage = "Updating"
	WorkspaceStageError       WorkspaceStage = "Errored"
	WorkspaceStageTerminating WorkspaceStage = "Terminating"
	WorkspaceStageDeleted     WorkspaceStage = "Deleted"
)

type WorkspaceSpec struct {
	Branch BranchSpec `json:"branch"`

	// Collection of all the components that are required for this
	// workspace to deploy.
	Components []ComponentSpec `json:"components,omitempty"`

	// Defines all the environments that will be needed for this workspace
	Environments []EnvironmentSpec `json:"environments"`

	// Default tag for all the images that are build that don't
	// have a tag specified to them. If no value is set,
	// it will be created before the builds starts.
	// +optional
	Tag *string `json:"tag,omitempty"`
}

type BranchSpec struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type ServiceSpec struct {
	Ingress  string `json:"ingress,omitempty"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
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

	Stage WorkspaceStage `json:"stage"`

	// Builds are the unit of work associated for each of the builds
	// that are required for this workspace to launch. Builds are seeding
	// the Images as they complete.
	Builds []BuildReference `json:"builds,omitempty"`

	// Images are seeded by Builds as they are completed. It's
	// also possible for some services in a workspace to have images that don't
	// require a build (think database, etc.).
	Images map[string]BuildImage `json:"images,omitempty"`
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
