package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/releasehub-com/spot/builder/internal/buildkit"
	"github.com/releasehub-com/spot/builder/internal/k8s"
	"github.com/releasehub-com/spot/builder/internal/registries"
	"github.com/releasehub-com/spot/builder/internal/source"
	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/env"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	ctx := context.Background()
	logger := log.FromContext(ctx)
	spot.AddToScheme(scheme.Scheme)

	client, err := k8s.NewClient(ctx, &spot.GroupVersion)
	if err != nil {
		handleFatalErr(ctx, client, err)
	}

	build, err := client.GetBuild(ctx, strings.Split(os.Getenv("BUILD_REFERENCE"), "/"))
	if err != nil {
		handleFatalErr(ctx, client, err)
	}

	var src *source.Repository
	if err := client.MonitorCondition(ctx, build, spot.BuildConditionSource, func(ctx context.Context, _ *spot.Build) error {
		logger.Info("Configuring data for repository access")
		secrets, err := source.SecretsFromReader(strings.NewReader(os.Getenv("REPOSITORY_SECRETS")))
		if err != nil {
			return err
		}

		opts := source.RepositoryOpts{
			BuildContext: os.Getenv("REPOSITORY_CONTEXT"),
			Host:         os.Getenv("REPOSITORY_URL"),
			Reference:    plumbing.NewHashReference(plumbing.NewBranchReferenceName(os.Getenv("REPOSITORY_BRANCH")), plumbing.NewHash(os.Getenv("REPOSITORY_COMMIT"))),
			Secrets:      secrets,
		}

		src, err = source.Git(ctx, opts)
		return err
	}); err != nil {
		handleFatalErr(ctx, client, err)
	}

	logger.Info("Waiting for buildkitd to be ready")
	for {
		cmd := exec.Command("buildctl", "debug", "workers")
		if err := cmd.Run(); err != nil {
			time.Sleep(100 * time.Millisecond) // Hack. need something more deterministic at some point
			continue
		}
		break
	}
	logger.Info("Buildkit ready")

	var imageIndex v1.ImageIndex
	if err := client.MonitorCondition(ctx, build, spot.BuildConditionBuilding, func(ctx context.Context, build *spot.Build) error {
		imageIndex, err = buildkit.Build(ctx, src)
		return err
	}); err != nil {
		handleFatalErr(ctx, client, err)
	}

	logger.Info("Setting up credentials for the registries")
	keychain, err := registries.NewKeychain(strings.NewReader(os.Getenv("REGISTRY_SECRETS")))
	if err != nil {
		handleFatalErr(ctx, client, err)
	}

	if err := client.MonitorCondition(ctx, build, spot.BuildConditionRegistry, func(ctx context.Context, build *spot.Build) error {
		image, err := registries.Upload(ctx, imageIndex, env.GetString("IMAGE_URL", ""), keychain)
		build.Status.Image = image
		return err
	}); err != nil {
		handleFatalErr(ctx, client, err)
	}
}

// handle unrecoverable error by attempting to update the Build custom resource one last
// time and then panicking. This is the last chance to tell the operator the reason why this build is failing.
func handleFatalErr(ctx context.Context, client *k8s.Client, err error) {
	// Can't update the API if the client doesn't exist
	if client == nil {
		panic(err)
	}

	panic(err)
}
