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

package kubetest

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"

	gitops "github.com/kharf/declcd/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/vcs"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type Environment struct {
	ControlPlane          *envtest.Environment
	TestKubeClient        client.Client
	DynamicTestKubeClient *kube.ExtendedDynamicClient
	RepositoryManager     vcs.RepositoryManager
	Ctx                   context.Context
	clean                 func() error
}

func (env Environment) Stop() error {
	return env.clean()
}

type enabled bool

var _ Option = (*enabled)(nil)

func (opt enabled) apply(opts *options) {
	opts.enabled = bool(opt)
}

type vcsAuthSecret struct {
	projectName string
}

var _ Option = (*vcsAuthSecret)(nil)

func (opt *vcsAuthSecret) apply(opts *options) {
	opts.vcsAuthSecret = opt
}

type options struct {
	enabled       bool
	vcsAuthSecret *vcsAuthSecret
}

type Option interface {
	apply(*options)
}

func WithEnabled(isEnabled bool) enabled {
	return enabled(isEnabled)
}

func WithVCSAuthSecretFor(projectName string) *vcsAuthSecret {
	return &vcsAuthSecret{
		projectName: projectName,
	}
}

func StartKubetestEnv(t testing.TB, log logr.Logger, opts ...Option) *Environment {
	options := &options{
		enabled: true,
	}
	for _, o := range opts {
		o.apply(options)
	}

	if !options.enabled {
		return nil
	}

	ctrl.SetLogger(log)

	testEnv := &envtest.Environment{
		ErrorIfCRDPathMissing: false,
	}

	var err error
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatal(err)
	}

	err = gitops.AddToScheme(scheme.Scheme)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	testClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatal(err)
	}

	client, err := kube.NewExtendedDynamicClient(testEnv.Config)
	assert.NilError(t, err)

	nsStr := "test"
	declNs := corev1.Namespace{
		TypeMeta: v1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: nsStr,
		},
	}
	err = testClient.Create(ctx, &declNs)
	assert.NilError(t, err)

	if options.vcsAuthSecret != nil {
		sec := corev1.Secret{
			TypeMeta: v1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      vcs.SecretName(options.vcsAuthSecret.projectName),
				Namespace: nsStr,
			},
			Data: map[string][]byte{
				vcs.K8sSecretDataAuthType: []byte(vcs.K8sSecretDataAuthTypeSSH),
				vcs.SSHKey: []byte(`-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz
c2gtZWQyNTUxOQAAACDrGFnmApwnObDTPK8nepGtlPKhhrA1u6Ox2hD5LAq5+gAA
AIh1qzZ4das2eAAAAAtzc2gtZWQyNTUxOQAAACDrGFnmApwnObDTPK8nepGtlPKh
hrA1u6Ox2hD5LAq5+gAAAEDiqr5GEHcp1oHqJCNhc+LBYF9LDmuJ9oL0LUw5pYZy
9OsYWeYCnCc5sNM8ryd6ka2U8qGGsDW7o7HaEPksCrn6AAAAAAECAwQF
-----END OPENSSH PRIVATE KEY-----`),
				vcs.SSHPubKey: []byte("ssh-ed25519 AAAA"),
			},
		}
		err = testClient.Create(ctx, &sec)
		assert.NilError(t, err)
	}

	repositoryManger := vcs.NewRepositoryManager("test", client.DynamicClient(), log)

	return &Environment{
		ControlPlane:          testEnv,
		TestKubeClient:        testClient,
		DynamicTestKubeClient: client,
		RepositoryManager:     repositoryManger,
		Ctx:                   ctx,
		clean: func() error {
			cancel()
			return testEnv.Stop()
		},
	}
}

type FakeDynamicClient struct {
	Err error
}

var _ kube.Client[unstructured.Unstructured, unstructured.Unstructured] = (*FakeDynamicClient)(nil)

func (client *FakeDynamicClient) Apply(
	ctx context.Context,
	obj *unstructured.Unstructured,
	fieldManager string,
	opts ...kube.ApplyOption,
) error {
	return client.Err
}

func (client *FakeDynamicClient) Update(
	ctx context.Context,
	obj *unstructured.Unstructured,
	fieldManager string,
	opts ...kube.ApplyOption,
) error {
	return client.Err
}

func (client *FakeDynamicClient) Delete(ctx context.Context, obj *unstructured.Unstructured) error {
	return client.Err
}

func (client *FakeDynamicClient) Get(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	return nil, client.Err
}

func (client *FakeDynamicClient) RESTMapper() meta.RESTMapper {
	return nil
}
