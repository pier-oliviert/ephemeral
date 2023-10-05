package source

import (
	"encoding/json"
	"errors"
	"io"
)

var ErrSecretNotFound = errors.New("Secret not found")

type Secrets []Secret
type Secret struct {
	Host        string `json:"host"`
	AccessKey   string `json:"access_key"`
	SecretToken string `json:"secret_token"`
}

func SecretsFromReader(r io.Reader) (Secrets, error) {
	var secrets Secrets
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&secrets); err != nil {
		return nil, err
	}

	return secrets, nil
}

func (s Secrets) SecretForHost(host string) (*Secret, error) {
	var secret *Secret
	for _, sct := range s {
		if sct.Host == host {
			secret = &sct
			break
		}
	}

	if secret == nil {
		return nil, ErrSecretNotFound
	}

	return secret, nil
}
