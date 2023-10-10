package buildkit

import (
	"context"
	"encoding/json"
	"io"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Argument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Arguments []Argument

type Secret struct {
	Name  string `json:"name"`
	Value string `json:"value"`

	// Path is going to be set after the secret is parsed.
	Path string
}
type Secrets []Secret

func ParseAttributes(ctx context.Context, secretReader, argumentReader io.Reader) (Secrets, Arguments, error) {
	logger := log.FromContext(ctx)

	var secrets Secrets
	secretDecoder := json.NewDecoder(secretReader)

	logger.Info("Decoding Build Secrets")
	if err := secretDecoder.Decode(&secrets); err != nil {

		return nil, nil, err
	}

	var arguments Arguments
	argumentDecoder := json.NewDecoder(argumentReader)

	logger.Info("Decoding Build Arguments")
	if err := argumentDecoder.Decode(&arguments); err != nil {

		return nil, nil, err
	}

	return secrets, arguments, nil
}
