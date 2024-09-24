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

package vcs

import (
	"context"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/kharf/declcd/pkg/kube"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	K8sSecretDataAuthType    = "auth"
	K8sSecretDataAuthTypeSSH = "ssh"
	SSHKey                   = "identity"
	SSHPubKey                = "identity.pub"
	Token                    = "token"
)

// Auth methods for repository access.
type Auth struct {
	Method transport.AuthMethod

	// Personal Access Token with access rights to the VCS repository.
	// When empty, features like PR creation won't be possible.
	Token string
}

func GetAuth(
	ctx context.Context,
	kubeClient kube.Client[unstructured.Unstructured, unstructured.Unstructured],
	controllerNamespace string,
	projectName string,
) (*Auth, error) {
	authSecret, err := getAuthSecret(
		ctx,
		kubeClient,
		controllerNamespace,
		projectName,
	)
	if err != nil && k8sErrors.ReasonForError(err) != metav1.StatusReasonNotFound {
		return nil, err
	}

	var authMethod transport.AuthMethod
	var token string
	if authSecret != nil {
		switch string(authSecret.Data[K8sSecretDataAuthType]) {
		case "ssh":
			priv := authSecret.Data[SSHKey]
			public, err := ssh.NewPublicKeys("git", priv, "")
			if err != nil {
				return nil, err
			}
			authMethod = public

			token = string(authSecret.Data[Token])
		}
	}

	return &Auth{
		Method: authMethod,
		Token:  token,
	}, nil
}

func (config RepositoryConfigurator) createAuthSecret(
	ctx context.Context,
	projectName string,
	fieldManager string,
	depKey deployKey,
	persistToken bool,
) error {
	unstr := &unstructured.Unstructured{}
	unstr.SetName(SecretName(projectName))
	unstr.SetNamespace(config.controllerNamespace)
	unstr.SetKind("Secret")
	unstr.SetAPIVersion("v1")
	unstr.Object["data"] = map[string][]byte{
		SSHKey:                []byte(depKey.privateKeyOpenSSH),
		SSHPubKey:             []byte(depKey.publicKeyOpenSSH),
		K8sSecretDataAuthType: []byte(K8sSecretDataAuthTypeSSH),
	}

	if persistToken {
		unstr.Object["data"].(map[string][]byte)[Token] = []byte(config.token)
	}

	if _, err := config.kubeClient.Apply(ctx, unstr, fieldManager); err != nil {
		return err
	}

	return nil
}

func getAuthSecret(
	ctx context.Context,
	kubeClient kube.Client[unstructured.Unstructured, unstructured.Unstructured],
	controllerNamespace string,
	projectName string,
) (*v1.Secret, error) {
	unstr := &unstructured.Unstructured{}
	unstr.SetName(SecretName(projectName))
	unstr.SetNamespace(controllerNamespace)
	unstr.SetKind("Secret")
	unstr.SetAPIVersion("v1")

	unstr, err := kubeClient.Get(ctx, unstr)
	if err != nil {
		return nil, err
	}

	var sec v1.Secret
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstr.Object, &sec); err != nil {
		return nil, err
	}

	return &sec, nil
}
