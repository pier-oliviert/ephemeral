package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Workspace", func() {

	Context("Tags", func() {
		It("it needs that starts with an alphabetic character", func() {
			workspace := &Workspace{}
			workspace.Default()
			Expect(workspace.Spec.Tag).To(MatchRegexp("^[a-zA-Z].+"))
		})
	})

	Context("Finalizers", func() {

		It("should add the default finalizer if it doesn't exist", func() {
			workspace := &Workspace{}
			workspace.Default()
			Expect(len(workspace.Finalizers)).To(Equal(1))
			Expect(workspace.Finalizers[0]).To(Equal(WorkspaceFinalizer))
		})

	})

})
