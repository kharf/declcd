// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kharf/declcd/internal/manifest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/vcs"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrHelmInstallationUnsupported = errors.New("Helm installation not supported yet")
)

type InstallOptions struct {
	Branch   string
	Url      string
	Name     string
	Token    string
	Interval int
	Shard    string
}

type InstallAction struct {
	kubeClient       *kube.DynamicClient
	httpClient       *http.Client
	componentBuilder component.Builder
	projectRoot      string
}

func NewInstallAction(
	kubeClient *kube.DynamicClient,
	httpClient *http.Client,
	projectRoot string,
) InstallAction {
	return InstallAction{
		kubeClient:  kubeClient,
		projectRoot: projectRoot,
		httpClient:  httpClient,
	}
}

func (act InstallAction) Install(ctx context.Context, opts InstallOptions) error {
	declcdDir := filepath.Join(act.projectRoot, "declcd")
	projectFileName := filepath.Join(declcdDir, fmt.Sprintf("%s_project.cue", opts.Name))

	_, err := os.Stat(projectFileName)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if os.IsNotExist(err) {
		var projectBuf bytes.Buffer
		projectTmpl, err := template.New("").Parse(manifest.Project)
		if err != nil {
			return err
		}

		if err := projectTmpl.Execute(&projectBuf, map[string]interface{}{
			"Name":                opts.Name,
			"Namespace":           ControllerNamespace,
			"Branch":              opts.Branch,
			"PullIntervalSeconds": opts.Interval,
			"Shard":               opts.Shard,
			"Url":                 opts.Url,
		}); err != nil {
			return err
		}

		if err := os.WriteFile(projectFileName, projectBuf.Bytes(), 0666); err != nil {
			return err
		}
	}

	instances, err := act.componentBuilder.Build(
		component.WithPackagePath("./declcd"),
		component.WithProjectRoot(act.projectRoot),
	)
	if err != nil {
		return err
	}

	dag := component.NewDependencyGraph()
	if err := dag.Insert(instances...); err != nil {
		return err
	}

	instances, err = dag.TopologicalSort()
	if err != nil {
		return err
	}

	controllerName := getControllerName(opts.Shard)
	for _, instance := range instances {
		manifest, ok := instance.(*component.Manifest)
		if !ok {
			return ErrHelmInstallationUnsupported
		}

		if opts.Shard == manifest.Content.GetLabels()["declcd/shard"] {
			timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			if err := act.installObject(
				timeoutCtx,
				&manifest.Content,
				controllerName,
			); err != nil {
				return err
			}
		}
	}

	repoConfigurator, err := vcs.NewRepositoryConfigurator(
		ControllerNamespace,
		act.kubeClient,
		act.httpClient,
		opts.Url,
		opts.Token,
	)
	if err != nil {
		return err
	}

	if err := repoConfigurator.CreateDeployKeyIfNotExists(ctx, controllerName, opts.Name); err != nil {
		return err
	}

	return nil
}

func (act InstallAction) installObject(
	ctx context.Context,
	unstr *unstructured.Unstructured,
	fieldManager string,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := act.kubeClient.Apply(ctx, unstr, fieldManager); err != nil {
		if k8sErrors.IsNotFound(err) {
			time.Sleep(1 * time.Second)
			return act.installObject(ctx, unstr, fieldManager)
		}
		return err
	}

	return nil
}
