package buildkit

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"

	ecrLogin "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"k8s.io/utils/env"
)

const DOCKER_HUB_RELATIVE_PATH = "https://index.docker.io/v1"

type RegistryAuth struct {
	Registries map[Domain]Credential `json:"auths"`

	ecr       *ecrLogin.ECRHelper
	dockerHub *dockerHub
	path      string
}

type Domain string

type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewRegistryAuth(path string) (*RegistryAuth, error) {
	err := os.Mkdir(fmt.Sprint(path), 0777)
	if err != nil {
		return nil, err
	}

	return &RegistryAuth{
		Registries: make(map[Domain]Credential),
		ecr:        ecrLogin.NewECRHelper(),
		dockerHub:  &dockerHub{},
		path:       path,
	}, nil
}

// Locates the credentials in the pod for the URL
// and add the entry. If credential already exists for this
// URL, it will be replaced.
// Returns an error if no credential can be found for the provided URL.
func (a *RegistryAuth) Set(rawURL string) error {
	url, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	// Relative URL (ie. myuser/mydockerRepo) is exclusive
	// to DockerHub and local docker container.
	if !url.IsAbs() {

		local, err := env.GetBool("DOCKER_LOCAL", false)
		if err != nil {
			return err
		}

		if local {
			a.Registries[Domain(rawURL)] = a.dockerHub.Credential()
		} else {
			a.Registries[Domain(fmt.Sprintf("%s/%s", DOCKER_HUB_RELATIVE_PATH, rawURL))] = a.dockerHub.Credential()
		}

		return nil
	}

	username, password, err := a.ecr.Get(rawURL)
	if err != nil {
		return err
	}

	a.Registries[Domain(rawURL)] = Credential{
		Username: username,
		Password: password,
	}

	return nil
}

func (a *RegistryAuth) Store() error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}

	return os.WriteFile(fmt.Sprint(a.path, "/.docker/config.json"), []byte(data), fs.ModeAppend)
}

type dockerHub struct{}

func (dh *dockerHub) Credential() Credential {
	return Credential{
		Username: env.GetString("DOCKERHUB_USER", "dockerhub user not found"),
		Password: env.GetString("DOCKERHUB_PASSWORD", "dockerhub password not found"),
	}
}
