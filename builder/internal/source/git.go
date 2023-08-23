package source

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
)

type Repository struct {
	path string
	*git.Repository
}

func FromRepository(name, url, referenceName string) (*Repository, error) {
	repo := &Repository{
		path: fmt.Sprintf("%s/sources/%s", os.TempDir(), name),
	}

	if err := os.MkdirAll(repo.path, os.ModePerm); err != nil {
		return nil, err
	}

	var err error
	repo.Repository, err = git.PlainClone(repo.path, false, &git.CloneOptions{
		URL:          url,
		SingleBranch: true,
	})
	if err != nil {
		return nil, err
	}

	return repo, nil
}

func (r *Repository) Path() string {
	return r.path
}

func (r *Repository) Ref() (string, error) {
	ref, err := r.Head()
	if err != nil {
		return "", err
	}

	return ref.Hash().String(), nil
}
