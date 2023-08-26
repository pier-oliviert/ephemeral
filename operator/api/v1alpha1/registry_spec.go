package v1alpha1

type RegistrySpec struct {
	// URL is the complete URL that points to a registry.
	// The Images built by the Builder will be pushed to this registry.
	// If the registry is private, the service account that the builder runs in
	// needs to have write access to the registry.
	//
	// TODO: Add a SecretRef so that the Builder CRD can be provided a
	// secret for when a service account can't be set.
	URL string `json:"url"`
}
