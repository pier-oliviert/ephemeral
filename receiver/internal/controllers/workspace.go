package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	spot "github.com/releasehub-com/spot/operator/api/v1alpha1"
)

type Workspace struct {
	Client rest.Interface
}

type WorkspaceRequest struct {
	Project string        `json:"project"`
	State   string        `json:"state"`
	Branch  BranchRequest `json:"branch"`
}

type BranchRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Hash string `json:"hash"`
	Ref  string `json:"ref"`
}

func (w *Workspace) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	var project spot.Project
	var workspaceRequest WorkspaceRequest

	if request.Method != "POST" {
		return
	}

	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(&workspaceRequest); err != nil {
		fmt.Println(err)
	}

	fmt.Println(workspaceRequest)

	r := w.Client.Get().Resource("projects").Namespace("spot-system").Name(workspaceRequest.Project)
	fmt.Println("Path: ", r.URL())

	result := r.Do(context.TODO())

	if err := result.Error(); err != nil {
		fmt.Println("Error trying to get the project CRD: ", err)
		return
	}

	if err := result.Into(&project); err != nil {
		fmt.Println("Error trying format the receiver: ", err)
		return
	}

	err := w.workspace(&project, &workspaceRequest)

	if err != nil {
		fmt.Println("Error trying to get the list of workspaces: ", err)
	}
}

func (w *Workspace) workspace(project *spot.Project, request *WorkspaceRequest) error {
	if request.State == "open" {
		var workspaces spot.Workspace

		result := w.Client.Get().Resource("workspaces").Namespace("spot-system").Name(request.Branch.Ref).Do(context.Background())
		if err := result.Error(); err != nil {
			if errors.IsNotFound(err) {
				fmt.Println("Not Found, creating a new workspace: ")
				return w.createWorkspace(project, request)
			}
			return err
		}

		err := result.Into(&workspaces)
		return err
	}

	result := w.Client.Delete().Resource("workspaces").Namespace("spot-system").Name(request.Branch.Ref).Do(context.Background())
	if err := result.Error(); err != nil {
		if errors.IsNotFound(err) {
			fmt.Println("Not Found, nothing to do.")
			return nil
		}
		return err
	}
	return nil
}

func (w *Workspace) createWorkspace(project *spot.Project, request *WorkspaceRequest) error {
	workspace := &spot.Workspace{
		ObjectMeta: v1.ObjectMeta{
			Name:      request.Branch.Name,
			Namespace: project.Namespace,
		},
		Spec: spot.WorkspaceSpec{
			Components:   project.Spec.Template.Components,
			Environments: project.Spec.Template.Environments,
			Tag:          &request.Branch.Ref, // TODO: Need to figure this out, probably wants it in the BranchSpec.
		},
	}

	for i := 0; i < len(workspace.Spec.Components); i++ {
		component := workspace.Spec.Components[i]
		if component.Image.Repository != nil {
			component.Image.Repository.Ref = request.Branch.Ref
			component.Image.Tag = &request.Branch.Ref
		}
		workspace.Spec.Components[i] = component
	}

	workspace.Spec.Tag = &request.Branch.Ref

	err := w.Client.
		Post().
		Resource("workspaces").
		Namespace("spot-system").
		Body(workspace).
		Do(context.TODO()).
		Into(workspace)

	return err
}
