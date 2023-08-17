package buildkit

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auth", func() {
	Context("New", func() {
		It("configures private 3rd party integrations", func() {
			auth, err := NewRegistryAuth(os.TempDir())

			Expect(err).To(BeNil())
			Expect(auth.ecr).ToNot(Equal(nil))
			Expect(auth.dockerHub).ToNot(Equal(nil))
		})
	})
})
