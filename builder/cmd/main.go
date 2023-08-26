package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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
		panic(err)
	}

	src, err := source.FromGitURL("build.Name", os.Getenv("REPOSITORY_URL"), os.Getenv("REPOSITORY_REF"))
	if err != nil {
		panic(err)
	}

	logger.Info("Setting up credentials for the registries")
	auth, err := registries.NewAuth(fmt.Sprintf("%s/%s", os.Getenv("HOME"), ".docker"))
	if err != nil {
		panic(err)
	}

	auth.Set(os.Getenv("REGISTRY_URL"))
	if err := auth.Store(); err != nil {
		panic(err)
	}

	logger.Info("Waiting for buildkitd to be ready")
	for {
		cmd := exec.Command("buildctl", "debug", "workers")
		if err := cmd.Run(); err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	logger.Info("Buildkit ready")

	imageIndex, err := buildkit.Build(ctx, src)
	if err != nil {
		panic(err)
	}

	image, err := registries.Upload(imageIndex, env.GetString("REGISTRY_URL", ""))
	if err != nil {
		panic(err)
	}

	logger.Info("Fetching Build CRD")
	references := strings.Split(os.Getenv("BUILD_REFERENCE"), "/")
	if len(references) != 2 {
		panic(fmt.Sprintf("BUILD_REFERENCE is expected to have 2 components, had %d: %s", len(references), os.Getenv("BUILD_REFERENCE")))
	}

	var build spot.Build
	req := client.Get().Resource("builds").Namespace(references[0]).Name(references[1])
	result := req.Do(ctx)

	if err := result.Error(); err != nil {
		panic(fmt.Sprintf("Error trying to get the build CRD: %v", err))
	}

	err = result.Into(&build)
	if err != nil {
		panic(fmt.Sprintf("Error trying format the build: %v", err))
	}

	build.Status.Stage = spot.BuildStageDone
	build.Status.Image = image
	result = client.Put().Resource("builds").SubResource("status").Namespace(build.Namespace).Name(build.Name).Body(&build).Do(ctx)
	if err = result.Error(); err != nil {
		panic(fmt.Sprintf("Error updating build: %v", err))
	}
}
