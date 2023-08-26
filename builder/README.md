# Builder

## Context within the operator

The build is in charge of building the image and pushing it to a remote registry. Once the execution of the builder is started, it will be
in charge of monitoring its own progress as well as maintain the associated CRD's Condition. 

## Environment Variables

As the builder is executed as part of a CRD, the runtime configuration is set through environment variables within the Operator reconcile loop.

|Name|Description|
|--|----|
|BUILD_REFERENCE|The Build CRD that initiated the execution of this build.|
|REGISTRY_URL|URL that points to a remote registry where the image will be pushed|
|REPOSITORY_URL|The URL where the git repository is located|
|REPOSITORY_REF|URL that points to a remote registry where the image will be pushed|