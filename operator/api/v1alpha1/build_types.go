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
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BuildSpec struct {
	// RepositoryURL is the URL of the repository it plans to build
	RepositoryURL string `json:"repo_url,omitempty"`

	// Information about the image that's going to be built
	// For an image to be succesfully built, it needs to have
	// a RegistrySpec associated with it.
	Image ImageSpec `json:"image,omitempty"`
}

// BuildStatus defines the observed state of Build
type BuildStatus struct {
	Stage BuildStage `json:"stage"`

	// The Pod that will run the build logic
	// It will be in charge of updating the status
	// of this Build and store the BuildImage
	// when the image is ready.
	Pod *PodReference `json:"pod,omitempty"`

	// The Image will store information about the image that
	// was created by this build. This value is nil until
	// the stage reaches BuildStageDone
	Image *BuildImage `json:"image,omitempty"`
}

type BuildImage struct {
	Digest string `json:"digest,omitempty"`
	URL    string `json:"url,omitempty"`
}

// +kubebuilder:validation:Enum=Running;Done;Errored
type BuildStage string

const (
	BuildStageInitialized BuildStage = "" // TODO: I would prefer if it was a string, but I can't get the enum to default to this otherwise.
	BuildStageRunning     BuildStage = "Running"
	BuildStageDone        BuildStage = "Done"
	BuildStageError       BuildStage = "Errored"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.status.stage`

// Build is the Schema for the builds API
type Build struct {
	meta.TypeMeta   `json:",inline"`
	meta.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildSpec   `json:"spec,omitempty"`
	Status BuildStatus `json:"status,omitempty"`
}

func (b *Build) GetReference() BuildReference {
	return BuildReference{
		Namespace: b.Namespace,
		Name:      b.Name,
	}
}

func (b *Build) ImageURL() string {
	if b.Spec.Image.Registry != nil {
		return b.Spec.Image.Registry.URL
	}

	return b.Spec.Image.Name
}

//+kubebuilder:object:root=true

// BuildList contains a list of Build
type BuildList struct {
	meta.TypeMeta `json:",inline"`
	meta.ListMeta `json:"metadata,omitempty"`
	Items         []Build `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Build{}, &BuildList{})
}
