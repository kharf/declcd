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

package ocitest

import (
	"strings"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/kharf/declcd/pkg/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type FakeClient struct {
	mu       sync.Mutex
	WantTags map[string][]string
	WantErr  error

	ListTagCalls []string
}

type FakeImage struct{}

func (f *FakeImage) ConfigFile() (*v1.ConfigFile, error) {
	panic("unimplemented config file")
}

func (f *FakeImage) ConfigName() (v1.Hash, error) {
	panic("unimplemented config name")
}

func (f *FakeImage) Digest() (v1.Hash, error) {
	hash, _, err := v1.SHA256(strings.NewReader("hash"))
	return hash, err
}

func (f *FakeImage) LayerByDiffID(v1.Hash) (v1.Layer, error) {
	panic("unimplemented layer diff id")
}

func (f *FakeImage) LayerByDigest(v1.Hash) (v1.Layer, error) {
	panic("unimplemented layer by digest")
}

func (f *FakeImage) Layers() ([]v1.Layer, error) {
	panic("unimplemented layers")
}

func (f *FakeImage) Manifest() (*v1.Manifest, error) {
	return &v1.Manifest{
		Annotations: map[string]string{
			ocispec.AnnotationURL: "test",
		},
	}, nil
}

func (f *FakeImage) MediaType() (types.MediaType, error) {
	panic("unimplemented media type")
}

func (f *FakeImage) RawConfigFile() ([]byte, error) {
	panic("unimplemented config file")
}

func (f *FakeImage) RawManifest() ([]byte, error) {
	panic("unimplemented raw manifest")
}

func (f *FakeImage) Size() (int64, error) {
	panic("unimplemented size")
}

var _ v1.Image = (*FakeImage)(nil)

func (f *FakeClient) Image(tag string, opts ...oci.Option) (v1.Image, error) {
	return &FakeImage{}, nil
}

func (f *FakeClient) ListTags(repoName string, opts ...oci.Option) ([]string, error) {
	f.mu.Lock()
	f.ListTagCalls = append(f.ListTagCalls, repoName)
	f.mu.Unlock()

	if f.WantErr != nil {
		return nil, f.WantErr
	}

	return f.WantTags[repoName], nil
}

var _ oci.Client = (*FakeClient)(nil)
