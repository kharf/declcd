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

package component

import (
	"crypto/tls"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/kharf/declcd/internal/dnstest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/pkg/helm"
	_ "github.com/kharf/declcd/test/workingdir"
	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuilder_Build(t *testing.T) {
	testRoot, err := os.MkdirTemp("", "")
	assert.NilError(t, err)
	defer os.RemoveAll(testRoot)
	dnsServer, err := dnstest.NewDNSServer()
	assert.NilError(t, err)
	defer dnsServer.Close()

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	// set to to true globally as CUE for example uses the DefaultTransport
	http.DefaultTransport = transport
	cueRegistry, err := ocitest.StartCUERegistry(testRoot)
	assert.NilError(t, err)
	defer cueRegistry.Close()

	builder := NewBuilder()
	cwd, err := os.Getwd()
	assert.NilError(t, err)
	testCases := []struct {
		name              string
		projectRoot       string
		packagePath       string
		expectedInstances []Instance
		expectedErr       string
	}{
		{
			name:        "Success",
			projectRoot: path.Join(cwd, "test", "testdata", "build"),
			packagePath: "./infra/prometheus",
			expectedInstances: []Instance{
				&Manifest{
					ID: "prometheus___Namespace",
					Content: unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Namespace",
							"metadata": map[string]interface{}{
								"name":      "prometheus",
								"namespace": "",
							},
						},
					},
					Dependencies: []string{},
				},
				&Manifest{
					ID: "secret_prometheus__Secret",
					Content: unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Secret",
							"metadata": map[string]interface{}{
								"name":      "secret",
								"namespace": "prometheus",
							},
							"data": map[string]interface{}{
								"foo": []byte("bar"),
							},
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&helm.ReleaseComponent{
					ID: "test_prometheus_HelmRelease",
					Content: helm.ReleaseDeclaration{
						Name:      "{{.Name}}",
						Namespace: "prometheus",
						Chart: helm.Chart{
							Name:    "test",
							RepoURL: "oci://test",
							Version: "test",
						},
						Values: helm.Values{
							"autoscaling": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&helm.ReleaseComponent{
					ID: "test-secret-ref_prometheus_HelmRelease",
					Content: helm.ReleaseDeclaration{
						Name:      "{{.Name}}",
						Namespace: "prometheus",
						Chart: helm.Chart{
							Name:    "test",
							RepoURL: "oci://test",
							Version: "test",
							Auth: &helm.Auth{
								SecretRef: &helm.SecretRef{
									Name:      "test-secret-ref",
									Namespace: "namespace",
								},
							},
						},
						Values: helm.Values{
							"autoscaling": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&helm.ReleaseComponent{
					ID: "test-workload-identity_prometheus_HelmRelease",
					Content: helm.ReleaseDeclaration{
						Name:      "{{.Name}}",
						Namespace: "prometheus",
						Chart: helm.Chart{
							Name:    "test",
							RepoURL: "oci://test",
							Version: "test",
							Auth: &helm.Auth{
								WorkloadIdentity: &helm.WorkloadIdentity{
									Provider: "gcp",
								},
							},
						},
						Values: helm.Values{
							"autoscaling": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
			},
			expectedErr: "",
		},
		{
			name:              "MissingMetadata",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/metadatamissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingMetadataNameWithSchema",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/metadatanameschemamissing",
			expectedInstances: []Instance{},
			expectedErr:       "secret.id: invalid interpolation: cannot reference optional field: name (and 1 more errors)",
		},
		{
			name:              "MissingMetadataName",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/metadatanamemissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingApiVersion",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/apiversionmissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingKind",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/kindmissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "EmptyReleaseNameWithSchema",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/emptyreleasenamewithschema",
			expectedInstances: []Instance{},
			expectedErr:       "release.name: invalid value \"\" (does not satisfy strings.MinRunes(1))",
		},
		{
			name:              "EmptyReleaseChartNameWithSchema",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/emptyreleasechartnamewithschema",
			expectedInstances: []Instance{},
			expectedErr:       "release.chart.name: invalid value \"\" (does not satisfy strings.MinRunes(1))",
		},
		{
			name:              "EmptyReleaseChartVersionWithSchema",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/emptyreleasechartversionwithschema",
			expectedInstances: []Instance{},
			expectedErr:       "release.chart.version: invalid value \"\" (does not satisfy strings.MinRunes(1))",
		},
		{
			name:              "WrongPrefixReleaseChartUrlWithSchema",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/wrongprefixreleasecharturlwithschema",
			expectedInstances: []Instance{},
			expectedErr:       "release.chart.repoURL: 3 errors in empty disjunction: (and 3 more errors)",
		},
		{
			name:              "ConflictingChartAuth",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/conflictingchartauth",
			expectedInstances: []Instance{},
			expectedErr:       "release.chart.auth: 2 errors in empty disjunction: (and 2 more errors)",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			components, err := builder.Build(
				WithProjectRoot(tc.projectRoot),
				WithPackagePath(tc.packagePath),
			)
			if tc.expectedErr != "" {
				assert.ErrorContains(t, err, tc.expectedErr)
			} else {
				assert.NilError(t, err)
				assert.Assert(t, len(components) == len(tc.expectedInstances))
				for i, expected := range tc.expectedInstances {
					current := components[i]
					switch expected := expected.(type) {
					case *Manifest:
						current, ok := current.(*Manifest)
						assert.Assert(t, ok)
						assert.Equal(t, current.ID, expected.ID)
						assert.DeepEqual(t, current.Dependencies, expected.Dependencies)
						assert.DeepEqual(t, current.Content, expected.Content)
					case *helm.ReleaseComponent:
						current, ok := current.(*helm.ReleaseComponent)
						assert.Assert(t, ok)
						assert.Equal(t, current.ID, expected.ID)
						assert.DeepEqual(t, current.Content.Values, expected.Content.Values)
						assert.DeepEqual(t, current.Dependencies, expected.Dependencies)
					}

				}
			}
		})
	}
}
