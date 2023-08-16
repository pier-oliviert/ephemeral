package buildkit

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
)

func Build(ctx context.Context) (*spot.BuildImage, error) {
	repo := os.Getenv("REPOSITORY_URL")
	ref := os.Getenv("REPOSITORY_REF")
	registry := os.Getenv("IMAGE_URL")
	imageTag := os.Getenv("IMAGE_TAG")

	file, err := os.CreateTemp("/tmp", "build-manifest-*")
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "buildctl", "build", "--frontend", "dockerfile.v0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Args = append(cmd.Args, "--opt", fmt.Sprintf("context=%s#%s", repo, ref))
	cmd.Args = append(cmd.Args, "--output", fmt.Sprintf("type=image,name=%s:%s,push=true", registry, imageTag))
	cmd.Args = append(cmd.Args, "--metadata-file", file.Name())
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}

	// Passing an int64 to make is safe here as
	// it's practically impossible for the metadata file to exceed the upper bound of int.
	// We'd  have much bigger problem if we're truncating the content to max-int instad of max-in64.
	content := make([]byte, stat.Size())

	if _, err := file.Read(content); err != nil {
		return nil, err
	}

	return &spot.BuildImage{
		URL:      fmt.Sprint(registry, ":", imageTag),
		Metadata: string(content),
	}, nil
}
