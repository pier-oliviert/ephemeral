package buildkit

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Argument struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type Arguments []Argument

type Secret struct {
	Key   string `json:"key"`
	Value string `json:"value"`

	File *os.File
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

// Buildkit needs the secret to be stored to a file
// so the secret is going to be stored in a temporary file
func (s *Secret) Store() (string, error) {
	if s.File != nil {
		return s.File.Name(), nil
	}

	file, err := os.CreateTemp("", s.Key)
	if err != nil {
		return "", err
	}

	if _, err := file.WriteString(s.Value); err != nil {
		return "", err
	}

	return file.Name(), nil
}

func (s *Secret) RemoveFile() error {
	if s.File != nil {
		return os.Remove(s.File.Name())
	}

	return nil
}
