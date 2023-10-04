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
	Registry RegistrySpec `json:"registry,omitempty"`
}

type RepositorySpec struct {
	// Location of your Dockerfile within the repository.
	Dockerfile string `json:"dockerfile"`

	// It's the location for the content of your build within the repository.
	Context string `json:"context"`

	// URL of the repository
	URL string `json:"url"`

	// Reference Hash
	Reference GitReference `json:"reference"`
}

// Represents a reference that is used to checkout a repository
// for a given commit.
type GitReference struct {
	// Name refers to the name of the branch we're working off of.
	// It can be master/main or any valid branch present in the remote repository(git)
	Name string `json:"name"`

	// The Hash represents the commit SHA of the commit that needs to be checked out.
	Hash string `json:"hash"`
}

type RegistrySpec struct {
	// URL is the complete URL that points to a registry.
	// The Images built by the Builder will be pushed to this registry.
	// If the registry is private, the service account that the builder runs in
	// needs to have write access to the registry.
	//
	// DockerHub special case is also supported here. If the URL is not a valid URL,
	// it will be expected to be a DockerHub image.
	URL string `json:"url"`

	// Tag to use when deploying the image as part of the workspace.
	// If the tag is not set, it will try to search for a default. If the
	// `Tags` field is set, it will use the first tag in that list.
	// If the `Tags` field is not set either, this field will be set to `latest`
	// +optional
	Tag *string `json:"tag,omitempty"`

	// List of tags the image will be exported with to the registry.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Target is an optional field to specify what Target you want
	// to export with this build. This is only usable for build that supports
	// more than one target.
	// +optional
	Target *string `json:"target,omitempty"`
}
