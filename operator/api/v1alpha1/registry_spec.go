package v1alpha1

type RegistrySpec struct {
	URL string `json:"url"`
	// TODO: Not sure this is the way to go, might replace it
	Type string `json:"type,omitempty"`
}
