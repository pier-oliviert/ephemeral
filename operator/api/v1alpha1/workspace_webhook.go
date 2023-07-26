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
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var workspacelog = logf.Log.WithName("workspace-resource")
var ErrWorkflowFinalizerMissing = errors.New("workflow requires a finalizer for namespaces")
var ErrWorkflowTagMissing = errors.New("workflow requires a tag to be set")

const WorkspaceGeneratedTagLength = 6

func (r *Workspace) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-spot-release-com-v1alpha1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=spot.release.com,resources=workspaces,verbs=create,versions=v1alpha1,name=mworkspace.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Workspace{}

func (r *Workspace) Default() {
	if !r.isFinalizerPresent() {
		r.Finalizers = append(r.Finalizers, WorkspaceFinalizer)
	}

	if r.Spec.Tag == "" {
		// Tags need to start with an alphabetic character to be a valid DNS_LABEL
		// so the generator is prefixed with a 'w' for workspace.
		r.Spec.Tag = fmt.Sprintf("w%s", rand.String(WorkspaceGeneratedTagLength-1))
	}
}

//+kubebuilder:webhook:path=/validate-spot-release-com-v1alpha1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=spot.release.com,resources=workspaces,verbs=create;update,versions=v1alpha1,name=vworkspace.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Workspace{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Workspace) ValidateCreate() (admission.Warnings, error) {
	if !r.isFinalizerPresent() {
		return nil, ErrWorkflowFinalizerMissing
	}

	if r.Spec.Tag == "" {
		workspacelog.Info("tag validation failed, might be possible to recover (TODO)", "name", r.Name)
		return nil, ErrWorkflowTagMissing
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Workspace) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	// Workspace is about to be deleted, skip validation.
	if r.DeletionTimestamp != nil {
		return nil, nil
	}

	if !r.isFinalizerPresent() {
		return nil, ErrWorkflowFinalizerMissing
	}

	if r.Spec.Tag == "" {
		workspacelog.Info("tag validation failed, might be possible to recover (TODO)", "name", r.Name)
		return nil, ErrWorkflowTagMissing
	}

	return nil, nil
}

// ValidateDelete is not needed, just here to satisfy the interface.
// Change kubebuiler verbs above to "verbs=create;update;delete" if you want to enable deletion validation.
func (r *Workspace) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func (r *Workspace) isFinalizerPresent() bool {
	for _, f := range r.Finalizers {
		if f == WorkspaceFinalizer {
			return true
		}
	}
	return false
}
