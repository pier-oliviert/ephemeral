package registries

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	gcr "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Upload the given imageIndex to the registy at `url`
func Upload(ctx context.Context, index gcr.ImageIndex, url string, keychain Keychain) (*spot.BuildImage, error) {
	logger := log.FromContext(ctx)

	ref, err := name.ParseReference(url)
	if err != nil {
		return nil, err
	}

	logger.Info("Uploading the index", "reference", ref)

	if err := remote.WriteIndex(ref, index, remote.WithAuthFromKeychain(keychain)); err != nil {
		return nil, err
	}

	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}

	metadata, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}

	registry := os.Getenv("IMAGE_URL")
	imageTag := os.Getenv("IMAGE_TAG")

	return &spot.BuildImage{
		URL:      fmt.Sprint(registry, ":", imageTag),
		Metadata: string(metadata),
	}, nil
}
