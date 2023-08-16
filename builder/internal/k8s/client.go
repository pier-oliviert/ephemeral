package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

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
