package sources

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
)

// TODO: Remove hardcoded auth here and use credentials stored in k8s
// TODO: Replace exec.CommandContext with llb.Client
func HardCodedBuildFromGithub(ctx context.Context) (*spot.BuildImage, error) {
	payload := os.Getenv("REGISTRY_AUTH")
	repo := os.Getenv("REPOSITORY_URL")
	ref := os.Getenv("REPOSITORY_REF")
	registry := os.Getenv("IMAGE_URL")
	imageTag := os.Getenv("IMAGE_TAG")
	err := os.Mkdir(fmt.Sprint(os.Getenv("HOME"), "/.docker"), 0777)
	if err != nil {
		log.Println(err)
	}

	err = os.WriteFile(fmt.Sprint(os.Getenv("HOME"), "/.docker/config.json"), []byte(payload), fs.ModeAppend)
	if err != nil {
		return nil, err
	}

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

	// TODO: Parse the metadata file and return the information inside the BuildImage
	return &spot.BuildImage{
		URL:    fmt.Sprint(registry, ":", imageTag),
		Digest: "TODO: digest from manifest",
	}, nil
}
