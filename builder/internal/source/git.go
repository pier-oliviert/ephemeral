package source

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Repository struct {
	buildContext string
	path         string
	*git.Repository
}

type RepositoryOpts struct {
	BuildContext string
	Host         string

	Reference *plumbing.Reference

	Secrets Secrets
}

// FromGitURL returns a fully configured Repository that can be used to build
// an image. If the repository is private, the url needs to include the access
// token.
//
// The repo is always cloned from scratch and doesn't check if it exists.
func Git(ctx context.Context, opts RepositoryOpts) (*Repository, error) {
	var err error

	logger := log.FromContext(ctx)
	secret, err := opts.Secrets.SecretForHost(opts.Host)
	if err != nil {
		return nil, err
	}

	logger.Info("Cloning Git Repository with options", "options", opts)

	auth := &http.BasicAuth{
		Username: secret.AccessKey,
		Password: secret.SecretToken,
	}

	repo := &Repository{
		buildContext: opts.BuildContext,
		path:         fmt.Sprintf("%s/src", os.TempDir()),
	}

	if err := os.MkdirAll(repo.path, os.ModePerm); err != nil {
		return nil, err
	}

	repo.Repository, err = git.PlainClone(repo.path, false, &git.CloneOptions{
		URL:           opts.Host,
		SingleBranch:  true,
		ReferenceName: opts.Reference.Name(),
		Auth:          auth,
		NoCheckout:    true,
	})

	if err != nil {
		return nil, err
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	err = w.Checkout(&git.CheckoutOptions{
		Hash: opts.Reference.Hash(),
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

// BuildContext return the absolute path
// to the context set by the user.
func (r *Repository) BuildContext() string {
	return path.Join(r.path, r.buildContext)
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
