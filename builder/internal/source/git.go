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

// FromGitURL returns a fully configured Repository that can be used to build
// an image. If the repository is private, the url needs to include the access
// token.
//
// The repo is always cloned from scratch and doesn't check if it exists.
func FromGitURL(name, url, referenceName string) (*Repository, error) {
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

// Path returns the temporary path where
// the repository was cloned.
func (r *Repository) Path() string {
	return r.path
}

// Ref returns the git reference(https://git-scm.com/book/en/v2/Git-Internals-Git-References)
// that was used to clone this repository. It is unlikely that a valid cloned
// repo returns an error here as it's asking for the Head, which will point at the reference
// used, but if it does happen, it means the repository is in an unknown state and can't be used
// to generate an image
func (r *Repository) Ref() (string, error) {
	ref, err := r.Head()
	if err != nil {
		return "", err
	}

	return ref.Hash().String(), nil
}
