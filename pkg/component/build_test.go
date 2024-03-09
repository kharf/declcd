package component

import (
	"os"
	"path"
	"testing"

	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/pkg/helm"
	_ "github.com/kharf/declcd/test/workingdir"
	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuilder_Build(t *testing.T) {
	testRoot, err := os.MkdirTemp("", "")
	assert.NilError(t, err)
	ocitest.StartCUERegistry(t, testRoot)
	builder := NewBuilder()
	cwd, err := os.Getwd()
	assert.NilError(t, err)
	testCases := []struct {
		name              string
		projectRoot       string
		componentPath     string
		expectedInstances []Instance
		expectedErr       string
	}{
		{
			name:          "Success",
			projectRoot:   path.Join(cwd, "test", "testdata", "build"),
			componentPath: "./infra/prometheus",
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
								"foo": []byte("(enc;value omitted)"),
							},
						},
					},
					Dependencies: []string{"prometheus___Namespace"},
				},
				&HelmRelease{
					ID: "{{.Name}}_prometheus_HelmRelease",
					Content: helm.ReleaseDeclaration{
						Name:      "{{.Name}}",
						Namespace: "prometheus",
						Chart: helm.Chart{
							Name:    "test",
							RepoURL: "{{.RepoUrl}}",
							Version: "{{.Version}}",
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
			componentPath:     "./infra/metadatamissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingMetadataNameWithSchema",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/metadatanameschemamissing",
			expectedInstances: []Instance{},
			expectedErr:       "secret.content.metadata.name: cannot convert non-concrete value string & string (and 1 more errors)",
		},
		{
			name:              "MissingMetadataName",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/metadatanamemissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingApiVersion",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/apiversionmissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingKind",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/kindmissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingReleaseName",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/releasenamemissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingReleaseNamespace",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/releasenamespacemissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingReleaseChart",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/releasechartmissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingReleaseChartName",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/releasechartnamemissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingReleaseChartRepoURL",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/releasechartrepourlmissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
		{
			name:              "MissingReleaseChartVersion",
			projectRoot:       path.Join(cwd, "test", "testdata", "build"),
			componentPath:     "./infra/releasechartversionmissing",
			expectedInstances: []Instance{},
			expectedErr:       ErrMissingField.Error(),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			components, err := builder.Build(
				WithProjectRoot(tc.projectRoot),
				WithComponentPath(tc.componentPath),
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
					case *HelmRelease:
						current, ok := current.(*HelmRelease)
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
