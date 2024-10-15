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

package oci

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type basicAuthOpt struct {
	user     string
	password string
}

type options struct {
	auth basicAuthOpt
}

type Option func(opts *options)

func WithBasicAuth(user, password string) Option {
	return func(opts *options) {
		opts.auth = basicAuthOpt{
			user:     user,
			password: password,
		}
	}
}

type Client interface {
	ListTags(repoName string, opts ...Option) ([]string, error)
	Image(tag string, opts ...Option) (v1.Image, error)
}

func NewRepositoryClient(repoName string) (Client, error) {
	repository, err := name.NewRepository(repoName)
	if err != nil {
		return nil, err
	}

	return &repositoryClient{
		repo: repository,
	}, nil
}

type repositoryClient struct {
	repo name.Repository
}

func (d *repositoryClient) Image(tag string, opts ...Option) (v1.Image, error) {
	image, err := remote.Image(d.repo.Tag(tag), evalOpts(opts)...)
	if err != nil {
		return nil, err
	}

	return image, nil
}

func (d *repositoryClient) ListTags(repoName string, opts ...Option) ([]string, error) {
	remoteVersions, err := remote.List(d.repo, evalOpts(opts)...)
	if err != nil {
		return nil, err
	}

	return remoteVersions, nil
}

var _ Client = (*repositoryClient)(nil)

func evalOpts(opts []Option) []remote.Option {
	options := &options{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}

	return []remote.Option{
		remote.WithAuth(&authn.Basic{
			Username: options.auth.user,
			Password: options.auth.password,
		}),
	}
}
