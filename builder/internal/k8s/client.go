package k8s

import (
	"context"
	"fmt"
	"os"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type Client struct {
	*rest.RESTClient
}

// Task represents a function that wraps around a condition
// for a Build. The function can operate on any resources it needs to.
// If an error occur while executing the task, it is the function's responsibility
// to return it so that the function calling this task can set the condition accordingly.
// Read more about the use of a Task at MonitorCondition()
type Task func(context.Context, *spot.Build) error

// Create a new Client that can communicate with the k8s cluster.
// This client will use the pod's service account to connect to the cluster
// and so requires read-write-list permissions on the Build CRD.
func NewClient(ctx context.Context, groupVersion *schema.GroupVersion) (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()
	config.ContentConfig.GroupVersion = groupVersion
	config.APIPath = "/apis"

	client, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}

	return &Client{client}, nil
}

// Return a Build custom resource from the k8s cluster. The build holds all the information
// to be able to build an image.
func (c *Client) GetBuild(ctx context.Context, references []string) (*spot.Build, error) {
	if len(references) != 2 {
		return nil, fmt.Errorf("BUILD_REFERENCE is expected to have 2 components, had %d: %s", len(references), os.Getenv("BUILD_REFERENCE"))
	}

	var build spot.Build
	req := c.Get().Resource("builds").Namespace(references[0]).Name(references[1])
	result := req.Do(ctx)

	if err := result.Error(); err != nil {
		return nil, fmt.Errorf("error trying to get the build CRD: %v", err)
	}

	if err := result.Into(&build); err != nil {
		return nil, fmt.Errorf("error trying format the build: %v", err)
	}

	return &build, nil
}

// Monitor condition takes a conditionType and a Task. It will set the condition's status throughout the lifecycle of a task.
// Each task's lifecycle is identical from the condition status' perspective where it starts by
// updating the condition to be InProgress, once the task is finished, it will check if it returned any error.
// Depending on whether an error exist, the Condition's Status will either be Success or Error.
//
// It's important to understand that this function will make 3 PUT request to the REST API and this can't run in parallel.
func (c *Client) MonitorCondition(ctx context.Context, build *spot.Build, conditionType spot.BuildConditionType, fn Task) error {
	if condition := build.Status.GetCondition(conditionType); condition.Status != spot.ConditionInProgress {
		condition.Status = spot.ConditionInProgress
		build.Status.SetCondition(condition)

		if err := c.updateBuildStatus(ctx, build); err != nil {
			return err
		}
	}

	if err := fn(ctx, build); err != nil {
		condition := build.Status.GetCondition(conditionType)
		condition.Status = spot.ConditionError
		build.Status.SetCondition(condition)

		if err := c.updateBuildStatus(ctx, build); err != nil {
			return err
		}

		return err
	}

	condition := build.Status.GetCondition(conditionType)
	condition.Status = spot.ConditionSuccess
	build.Status.SetCondition(condition)

	if err := c.updateBuildStatus(ctx, build); err != nil {
		return err
	}

	return nil
}

func (c *Client) updateBuildStatus(ctx context.Context, build *spot.Build) error {
	result := c.Put().Resource("builds").SubResource("status").Namespace(build.Namespace).Name(build.Name).Body(&build).Do(ctx)
	if err := result.Error(); err != nil {
		return err
	}

	if err := result.Into(build); err != nil {
		return err
	}

	return nil
}
