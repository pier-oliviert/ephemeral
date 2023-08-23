package registries

import (
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	gcr "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
)

func Upload(index gcr.ImageIndex, url string) (*spot.BuildImage, error) {
	ref, err := name.ParseReference(url)
	if err != nil {
		return nil, err
	}

	if err := remote.WriteIndex(ref, index, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		return nil, err
	}

	registry := os.Getenv("IMAGE_URL")
	imageTag := os.Getenv("IMAGE_TAG")

	return &spot.BuildImage{
		URL:      fmt.Sprint(registry, ":", imageTag),
		Metadata: "",
	}, nil
}
