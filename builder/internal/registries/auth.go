package registries

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	ecrLogin "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"k8s.io/utils/env"
)

const AUTH_CONFIG_JSON = "config.json"

// Auth is the umbrella wrapping all the logic to extract the any authentication
// fields that the builder support. The authentication token origin can change quite a lot
// between location where the cluster runs. It should be noted that this is subject to change a
// lot as I get more exposure to the different environnments.
//
// Although it might be possible to support more than 1 container registry in the future,
// Auth currently only support one.
//
// The Auth struct also includes all supported platform in its current form, it's
// reasonable to think we'll use different target in the future
// so that the logic here can be easier to reason about while
// the binary stay small for each of the provider.
type Auth struct {
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

// Create a new auth structure
// The path is the location on the filesystem where the
// Auth struct will be serialized to.
// If the auth is to be used with the default
// registry keychain, it will expect the value
// to be stored at `~/.docker/`
//
// Note that the path here doesn't include the filename,
// the filename defaults to AUTH_CONFIG_JSON
//
// Returns an error if the path is empty or if
// the folder can't be created.
func NewAuth(path string) (*Auth, error) {
	if path == "" {
		return nil, errors.New("no path provided, default location for a path is ~/.docker")
	}

	err := os.MkdirAll(fmt.Sprint(path), os.ModeDir)
	if err != nil {
		return nil, err
	}

	return &Auth{
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
func (a *Auth) Set(rawURL string) error {

	curi := newContainerURI(rawURL)

	switch curi.Provider {
	case "amazonaws":
		username, password, err := a.ecr.Get(rawURL)
		if err != nil {
			return err
		}

		a.Registries[Domain(rawURL)] = Credential{
			Username: username,
			Password: password,
		}
	default:
		// TODO: Make this supports both dockerhub and docker
		a.Registries[Domain(rawURL)] = a.dockerHub.Credential()
	}

	return nil
}

// Serialize and persist the content of the Auth struct
// to the filesystem.
// The location where the auth is serialized to is the concatenation of
// the path provided when creating the struct with the constant AUTH_CONFIG_JSON
//
// Returns an error if the auth can't be marshalled or if there was an error
// writing to the file.
func (a *Auth) Store() error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}

	return os.WriteFile(fmt.Sprint(a.path, "/", AUTH_CONFIG_JSON), []byte(data), fs.ModeAppend)
}

type dockerHub struct{}

func (dh *dockerHub) Credential() Credential {
	return Credential{
		Username: env.GetString("DOCKERHUB_USER", "dockerhub user not found"),
		Password: env.GetString("DOCKERHUB_PASSWORD", "dockerhub password not found"),
	}
}

type containerURI struct {
	Provider string
	Registry string
}

func newContainerURI(url string) containerURI {
	cURI := containerURI{}

	if strings.Contains(url, "amazonaws") {
		cURI.Provider = "amazonaws"
		cURI.Registry = url
	}

	return cURI
}
