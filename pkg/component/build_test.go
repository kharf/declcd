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
	"github.com/kharf/declcd/pkg/kube"
	_ "github.com/kharf/declcd/test/workingdir"
	"go.uber.org/goleak"
	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuilder_Build(t *testing.T) {
	defer goleak.VerifyNone(
		t,
	)

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
					Content: ExtendedUnstructured{
						Unstructured: &unstructured.Unstructured{
							Object: map[string]any{
								"apiVersion": "v1",
								"kind":       "Namespace",
								"metadata": map[string]any{
									"name": "prometheus",
								},
							},
						},
						Metadata: &FieldMetadata{
							IgnoreAttr: kube.OnConflict,
						},
						AttributeInfo: AttributeInfo{
							HasIgnoreConflictAttributes: true,
						},
					},
					Dependencies: []string{},
				},
				&Manifest{
					ID: "secret_prometheus__Secret",
					Content: ExtendedUnstructured{
						Unstructured: &unstructured.Unstructured{
							Object: map[string]any{
								"apiVersion": "v1",
								"kind":       "Secret",
								"metadata": map[string]any{
									"name":      "secret",
									"namespace": "prometheus",
								},
								"data": map[string]any{
									"foo": []byte("bar"),
								},
							},
						},
						Metadata: &MetadataNode{
							"data": &MetadataNode{
								"foo": &FieldMetadata{
									IgnoreAttr: kube.OnConflict,
								},
							},
						},
						AttributeInfo: AttributeInfo{
							HasIgnoreConflictAttributes: true,
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&Manifest{
					ID: "prometheus_prometheus_apps_Deployment",
					Content: ExtendedUnstructured{
						Unstructured: &unstructured.Unstructured{
							Object: map[string]any{
								"apiVersion": "apps/v1",
								"kind":       "Deployment",
								"metadata": map[string]any{
									"name":      "prometheus",
									"namespace": "prometheus",
								},
								"spec": map[string]any{
									"replicas": int64(1),
									"selector": map[string]any{
										"matchLabels": map[string]any{
											"app": "prometheus",
										},
									},
									"template": map[string]any{
										"metadata": map[string]any{
											"labels": map[string]any{
												"app": "prometheus",
											},
										},
										"spec": map[string]any{
											"containers": []any{
												map[string]any{
													"name":  "prometheus",
													"image": "prometheus:1.14.2",
													"ports": []any{
														map[string]any{
															"containerPort": int64(
																80,
															),
														},
													},
												},
												map[string]any{
													"name":  "sidecar",
													"image": "sidecar:1.14.2",
													"ports": []any{
														map[string]any{
															"containerPort": int64(
																80,
															),
														},
													},
												},
											},
										},
									},
								},
							},
						},
						Metadata: &MetadataNode{
							"spec": &MetadataNode{
								"replicas": &FieldMetadata{
									IgnoreAttr: kube.OnConflict,
								},
								"template": &MetadataNode{
									"spec": &MetadataNode{
										"containers": &FieldMetadata{
											IgnoreAttr: kube.OnConflict,
										},
									},
								},
							},
						},
						AttributeInfo: AttributeInfo{
							HasIgnoreConflictAttributes: true,
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&Manifest{
					ID: "prometheus_prometheus_rbac.authorization.k8s.io_Role",
					Content: ExtendedUnstructured{
						Unstructured: &unstructured.Unstructured{
							Object: map[string]any{
								"apiVersion": "rbac.authorization.k8s.io/v1",
								"kind":       "Role",
								"metadata": map[string]any{
									"name":      "prometheus",
									"namespace": "prometheus",
								},
								"rules": []any{
									map[string]any{
										"apiGroups": []any{"coordination.k8s.io"},
										"resources": []any{"leases"},
										"verbs": []any{
											"get",
											"create",
											"update",
										},
									},
									map[string]any{
										"apiGroups": []any{""},
										"resources": []any{"events"},
										"verbs": []any{
											"create",
											"patch",
										},
									},
								},
							},
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&helm.ReleaseComponent{
					ID: "test_prometheus_HelmRelease",
					Content: helm.ReleaseDeclaration{
						Name:      "test",
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
						CRDs: helm.CRDs{
							AllowUpgrade: false,
						},
						Patches: &helm.Patches{
							Unstructureds: map[string]kube.ExtendedUnstructured{
								"apps/v1-Deployment-prometheus-test": {
									Unstructured: &unstructured.Unstructured{
										Object: map[string]interface{}{
											"apiVersion": "apps/v1",
											"kind":       "Deployment",
											"metadata": map[string]any{
												"name":      "test",
												"namespace": "prometheus",
											},
											"spec": map[string]any{
												"replicas": int64(1),
											},
										},
									},
									Metadata: &MetadataNode{
										"spec": &MetadataNode{
											"replicas": &FieldMetadata{
												IgnoreAttr: kube.OnConflict,
											},
										},
									},
									AttributeInfo: AttributeInfo{
										HasIgnoreConflictAttributes: true,
									},
								},
								"apps/v1-Deployment-prometheus-hello": {
									Unstructured: &unstructured.Unstructured{
										Object: map[string]interface{}{
											"apiVersion": "apps/v1",
											"kind":       "Deployment",
											"metadata": map[string]any{
												"name":      "hello",
												"namespace": "prometheus",
											},
											"spec": map[string]any{
												"replicas": int64(2),
												"template": map[string]any{
													"spec": map[string]any{
														"containers": []any{
															map[string]any{
																"name":  "prometheus",
																"image": "prometheus:1.14.2",
																"ports": []any{
																	map[string]any{
																		"containerPort": int64(
																			80,
																		),
																	},
																},
															},
															map[string]any{
																"name":  "sidecar",
																"image": "sidecar:1.14.2",
																"ports": []any{
																	map[string]any{
																		"containerPort": int64(
																			80,
																		),
																	},
																},
															},
														},
													},
												},
											},
										},
									},
									Metadata: &MetadataNode{
										"spec": &MetadataNode{
											"replicas": &FieldMetadata{
												IgnoreAttr: kube.OnConflict,
											},
											"template": &MetadataNode{
												"spec": &MetadataNode{
													"containers": &FieldMetadata{
														IgnoreAttr: kube.OnConflict,
													},
												},
											},
										},
									},
									AttributeInfo: AttributeInfo{
										HasIgnoreConflictAttributes: true,
									},
								},
							},
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&helm.ReleaseComponent{
					ID: "test-secret-ref_prometheus_HelmRelease",
					Content: helm.ReleaseDeclaration{
						Name:      "test-secret-ref",
						Namespace: "prometheus",
						Chart: helm.Chart{
							Name:    "test",
							RepoURL: "oci://test",
							Version: "test",
							Auth: &helm.Auth{
								SecretRef: &helm.SecretRef{
									Name:      "secret",
									Namespace: "namespace",
								},
							},
						},
						Values: helm.Values{},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&helm.ReleaseComponent{
					ID: "test-workload-identity_prometheus_HelmRelease",
					Content: helm.ReleaseDeclaration{
						Name:      "test-workload-identity",
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
			name:              "MissingID",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			packagePath:       "./infra/idmissing",
			expectedInstances: []Instance{},
			expectedErr:       "secret: field not found: id",
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
			expectedErr:       "secret.id: invalid interpolation: cannot reference optional field: name",
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
		{
			name:        "AllowCRDsUpgrade",
			projectRoot: path.Join(cwd, "test", "testdata", "build"),
			packagePath: "./infra/allowcrdsupgrade",
			expectedInstances: []Instance{
				&helm.ReleaseComponent{
					ID: "test_test_HelmRelease",
					Content: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "test",
						Chart: helm.Chart{
							Name:    "test",
							RepoURL: "http://test",
							Version: "test",
						},
						Values: helm.Values{
							"autoscaling": map[string]interface{}{
								"enabled": true,
							},
						},
						CRDs: helm.CRDs{
							AllowUpgrade: true,
						},
					},
					Dependencies: []string{},
				},
			},
			expectedErr: "",
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
						assert.DeepEqual(t, current, expected)
					}
				}
			}
		})
	}
}
