# spot [![Go](https://github.com/releasehub-com/spot/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/releasehub-com/spot/actions/workflows/go.yml)
Managing ephemeral environment in your kubernetes cluster

## Development

### Podman machine

The default machine might not have enough CPU to run the whole environment as many pods needs to run and each of them have a resource quotas set. By default on OSX, the machine only has 1 CPU. It's preferrable to have 4 to make sure everything works fine.

```sh
podman machine init --cpus=4
```

You’ll need a Kubernetes cluster to run against. It's recommended to use [KIND](https://sigs.k8s.io/kind) as it was created to generate cluster rapidly with configuration files and such a config file exists in this repo to help you get started.

### KIND

It is recommended to create a cluster using the configuration file present in this repository.

```sh
kind create cluster --config kind.config.yaml
```

### Dependencies

For the operator to work properly, [Cert-manager](https://cert-manager.io/docs/) and [NGINX controller](https://kubernetes.github.io/ingress-nginx/deploy/) need to be running. You can follow the deployment guide provided in their documentation or you can
use the following commands to install them in your KIND cluster.

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
```

### Building from source

Each subproject can be deployed to the KIND cluster the same way. First, the project needs to be build and published to the local docker environment using a `$TAG` of your choosing, in the example below, the tag is `operator`. Once the build is completed,
it can be uploaded to the KIND cluster by using `kind load docker-image $TAG`.

```sh
export TAG=operator
cd operator/
docker build -t $TAG .
kind load docker-image $TAG
```

### Installing CRDs on the cluster

```sh
cd operator/
make generate
make manifest
make deploy
```

### Admission Webhooks

Admission webhooks are only supported if the operator is running *inside* a cluster. This means that if you run the operator through `make run` it will raise an error on startup. You can disable the admission webhooks from the operator by using an environment variable.

**Note:** If you plan on running the operator within the KIND cluster, this step is not needed.

```sh
DISABLE_WEBHOOKS=true make run
```


### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller from the cluster:

```sh
make undeploy
```

## Contributing

### Admission webhook certificates

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

