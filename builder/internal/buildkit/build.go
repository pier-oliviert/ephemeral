package buildkit

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	gcr "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/releasehub-com/spot/builder/internal/source"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var ImagePath = fmt.Sprintf("%s/%s", os.TempDir(), "image")

// Build the repository into an ImageIndex (OCI Standard)
// The context is set around the repository which means it needs to be
// present in the filesystem.
//
// The build execute buildkit as a system command directly and
// pipes both STDOUT and STDERR to their respective file descriptor.
//
// The error that returns from Build is any error that is returned from the buildkit
// process.
//
// The ImageIndex is generated from go-containerregistry and is a valid
// OCI ImageIndex that can be exported to any container registry.
func Build(ctx context.Context, repo *source.Repository, secrets Secrets, arguments Arguments) (gcr.ImageIndex, error) {
	logger := log.FromContext(ctx)

	logger.Info("Starting a build from a Repo", "Path", repo.BuildContext())

	cmd := exec.CommandContext(ctx, "buildctl", "build", "--frontend", "dockerfile.v0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Args = append(cmd.Args, "--local", fmt.Sprintf("context=%s", repo.BuildContext()))
	cmd.Args = append(cmd.Args, "--local", fmt.Sprintf("dockerfile=%s", repo.BuildContext()))
	cmd.Args = append(cmd.Args, "--output", fmt.Sprintf("type=oci,dest=%s,tar=false", ImagePath))

	for _, arg := range arguments {
		cmd.Args = append(cmd.Args, "--build-arg", fmt.Sprintf("%s=%s", arg.Key, arg.Value))
	}

	for _, secret := range secrets {
		path, err := secret.Store()
		if err != nil {
			return nil, err
		}

		logger.Info("Storing secret at a temporary path", "Path", path)
		cmd.Args = append(cmd.Args, "--secret", fmt.Sprintf("id=%s,src=%s", secret.Key, path))
	}

	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	return layout.ImageIndexFromPath(ImagePath)
}
