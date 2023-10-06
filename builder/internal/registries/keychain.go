package registries

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/authn"
)

// Keychain is the umbrella wrapping all the logic to extract the any authentication
// fields that the builder support. The authentication token origin can change quite a lot
// between location where the cluster runs. It should be noted that this is subject to change a
// lot as I get more exposure to the different environnments.
type Keychain map[Domain]Credential

type Domain string

type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Create a new Keychain
//
// Keychain can be used with the container registry
// as a way to authenticate to any private registry
//
// Currently, the logic expects the user/password to already be set up
// for the registry provider.
// The Keychain is created off the JSON payload from the reader
// and needs to follow the structure of the private struct define in the
// method.
func NewKeychain(r io.Reader) (Keychain, error) {
	type cred struct {
		Host        string `json:"host"`
		AccessKey   string `json:"access_key"`
		SecretToken string `json:"secret_token"`
	}
	var creds []cred

	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&creds); err != nil {
		return nil, err
	}

	kc := Keychain{}

	for _, c := range creds {
		kc[Domain(c.Host)] = Credential{
			Username: c.AccessKey,
			Password: c.SecretToken,
		}
	}

	return kc, nil
}

// Resolve returns an Authenticator that will be used by the container registry
// to authenticate the session.
func (kc Keychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	host := target.RegistryStr()
	credential, ok := kc[Domain(host)]

	if !ok {
		return nil, fmt.Errorf("couldn't find credentials for the host: %s", host)
	}

	return authn.FromConfig(authn.AuthConfig{
		Username: credential.Username,
		Password: credential.Password,
	}), nil
}
