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
	// Information about the image that's going to be built
	// For an image to be succesfully built, it needs to have
	// a RegistrySpec associated with it.
	Image ImageSpec `json:"image,omitempty"`
}

// BuildStatus defines the observed state of Build
type BuildStatus struct {
	// Phase is a composite of the conditions. It's main use is to display
	// the general state of the Build. This value is derived from the BuildConditions.
	// There are two different outcome for this build,
	// - Success: Before the phase shift to success, the `Image` will be set with and available.
	// - Error: At least one condition failed and some information can be found within the Condition. Additionally,
	//          it's possible that there's more information available in the events stream.
	Phase BuildPhase `json:"phase"`

	// Set of conditions that the build manages. For a build
	// to be successful and completed, all the conditions in this set
	// are required to have a status of ConditionSuccess.
	// A build is considered to have failed if at least one
	// of the condition in this list is marked as ConditionError
	Conditions BuildConditions `json:"conditions"`

	// The Pod that will run the build logic
	// It will be in charge of updating the status
	// of this Build and store the BuildImage
	// when the image is ready.
	Pod *Reference `json:"pod,omitempty"`

	// The Image will store information about the image that
	// was created by this build. This value is nil until
	// the stage reaches BuildStageDone
	Image *BuildImage `json:"image,omitempty"`
}

// Retrieve a *copy* of the condition if it already exists for the given type. If the condition
// doesn't exist, it will create a new one. In order to persist the condition on the status stack,
// the condition needs to be applied by calling `SetCondition(condition)`
func (bs *BuildStatus) GetCondition(bct BuildConditionType) BuildCondition {
	for _, c := range bs.Conditions {
		if c.Type == bct {
			return c
		}
	}

	return BuildCondition{
		Type:   bct,
		Status: ConditionInitialized,
	}
}

// Set the condition in the condition stack. If a condition for the given type already
// exists, it will be overwritten by this new condition. If it doesn't exist, it will be appended
// to the stack.
// For the condition to be persisted, the BuildStatus needs to be committed to the API. The
// LastTransitionTime is set here and represent the time that the reconciler set the condition in
// memory, not to be confused with the time the condition was committed to the database.
func (bs *BuildStatus) SetCondition(condition BuildCondition) {
	inserted := false

	if condition.LastTransitionTime.IsZero() {
		condition.LastTransitionTime = meta.Now()
	}

	for i, c := range bs.Conditions {
		if c.Type == condition.Type {
			bs.Conditions[i] = condition
			inserted = true
			break
		}
	}

	if !inserted {
		bs.Conditions = append(bs.Conditions, condition)
	}
}

type BuildConditions []BuildCondition

// Return a BuildPhase that represent the current derivation
// off the current conditions. If there's no conditions present
// in the BuildConditions, the phase will defaults to BuildPhaseDone
func (bc BuildConditions) Phase() BuildPhase {
	completed := true

	for _, cond := range bc {
		if cond.Status == ConditionError {
			return BuildPhaseError
		}

		if cond.Status != ConditionSuccess {
			completed = false
		}
	}

	if completed {
		return BuildPhaseDone
	}

	return BuildPhaseRunning
}

type BuildCondition struct {
	// Type is the name of the condition. Conceptually this represents a task in the process
	// of a Build.
	Type BuildConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=BuildConditionType"`

	// Status of the condition.
	// Can be In Progress, Error, Success.
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`

	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime meta.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,4,opt,name=lastTransitionTime"`
}

type BuildConditionType string

const (
	BuildConditionDeployPod BuildConditionType = "Pod Deployment"
	BuildConditionSource    BuildConditionType = "Retrieving Source"
	BuildConditionBuilding  BuildConditionType = "Building Image"
	BuildConditionRegistry  BuildConditionType = "Uploading Build to remote registry"
)

// +kubebuilder:validation:Enum=Running;Done;Errored
type BuildPhase string

const (
	BuildPhaseInitialized BuildPhase = ""        // TODO: Remove this when I can figure out how to set a default value.
	BuildPhaseRunning     BuildPhase = "Running" // TODO: Make this the default when the above is removed.
	BuildPhaseDone        BuildPhase = "Done"
	BuildPhaseError       BuildPhase = "Errored"
)

type BuildImage struct {
	Metadata string `json:"metadata,omitempty"`
	URL      string `json:"url,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Build is the Schema for the builds API
type Build struct {
	meta.TypeMeta   `json:",inline"`
	meta.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildSpec   `json:"spec,omitempty"`
	Status BuildStatus `json:"status,omitempty"`
}

func (b *Build) GetReference() Reference {
	return Reference{
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
