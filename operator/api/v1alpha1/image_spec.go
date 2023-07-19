package v1alpha1

type ImageSpec struct {
	// Repository information is passed down to buildkit
	// as instruction on how to proceed with the repository.
	// The image will be build from source if the `Repository` is set.
	//+optional
	Repository *RepositorySpec `json:"repository,omitempty"`

	// Registry is where all the information for the container registry
	// lives. It needs to be properly configured for the build to
	// be pushed successfully. A build is pushed to the registry only
	// if the `RepositoryContext` exists with this `Registry`
	Registry *RegistrySpec `json:"registry,omitempty"`

	// Tag is what will be used to tag the image once it's
	// pushed to the container's registry (ecr, etc.)
	// If no tag is set, it will use the workspace tag
	// This can be useful if a workspace builds multiple images
	// and each of the images will be tagged the same value.
	// Defaults to latest
	Tag *string `json:"tag,omitempty"`

	// Name of the image. If the image is not an official
	// one and a URL needs to be provided, `RegistrySpec`
	// needs to provide that URL.
	Name string `json:"name"`
}

type RepositorySpec struct {
	// Location of your Dockerfile within the repository.
	Dockerfile string `json:"dockerfile"`

	// It's the location for the content of your build within the repository.
	Context string `json:"context"`

	// URL of the repository
	URL string `json:"url"`

	// Hash of the commit to build (usually a sha)
	Hash string `json:"hash"`

	// Reference is usually a branch.
	Ref string `json:"ref"`
}
