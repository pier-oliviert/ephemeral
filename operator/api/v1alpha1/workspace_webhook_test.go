package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Workspace", func() {

	Context("Finalizers", func() {

		It("should add the default finalizer if it doesn't exist", func() {
			workspace := &Workspace{}
			workspace.Default()
			Expect(len(workspace.Finalizers)).To(Equal(1))
			Expect(workspace.Finalizers[0]).To(Equal(WorkspaceFinalizer))
		})

	})

})
