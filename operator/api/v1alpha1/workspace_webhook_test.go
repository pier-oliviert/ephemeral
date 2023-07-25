package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Workspace", func() {

	// Add Tests for OpenAPI validation (or additonal CRD features) specified in
	// your API definition.
	// Avoid adding tests for vanilla CRUD operations because they would
	// test Kubernetes API server, which isn't the goal here.
	Context("Finalizers", func() {

		It("should add the default finalizer if it doesn't exist", func() {
			workspace := &Workspace{}
			workspace.Default()
			Expect(len(workspace.Finalizers)).To(Equal(1))
			Expect(workspace.Finalizers[0]).To(Equal(WorkspaceFinalizer))
		})

	})

})
