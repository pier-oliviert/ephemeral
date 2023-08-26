package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// Create a new Client that can communicate with the k8s cluster.
// This client will use the pod's service account to connect to the cluster
// and so requires read-write-list permissions on the Build CRD.
func NewClient(ctx context.Context, groupVersion *schema.GroupVersion) (*rest.RESTClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()
	config.ContentConfig.GroupVersion = groupVersion
	config.APIPath = "/apis"

	return rest.RESTClientFor(config)
}
