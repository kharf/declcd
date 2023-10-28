package project

import (
	"os"
	"path"
	"testing"

	"github.com/kharf/declcd/pkg/helm"
	_ "github.com/kharf/declcd/test/workingdir"
	"gotest.tools/v3/assert"
)

func TestManifestInstanceBuilder_Build(t *testing.T) {
	builder := NewComponentBuilder()
	cwd, err := os.Getwd()
	assert.NilError(t, err)
	projectRoot := path.Join(cwd, "test", "testdata", "simple")
	component, err := builder.Build(WithProjectRoot(projectRoot), WithComponentPath("./infra/prometheus"))
	assert.NilError(t, err)
	componentManifests := component.Manifests
	assert.Assert(t, len(componentManifests) == 2)
	namespace := componentManifests[0].Object
	assert.Equal(t, namespace["apiVersion"], "v1")
	assert.Equal(t, namespace["kind"], "Namespace")
	nsMetadata := namespace["metadata"].(map[string]interface{})
	ns := "prometheus"
	assert.Equal(t, nsMetadata["name"], ns)
	releases := component.HelmReleases
	assert.Assert(t, len(releases) == 1)
	release := releases[0]
	assert.Equal(t, release.Name, "{{.Name}}")
	assert.Equal(t, release.Namespace, ns)
	assert.Assert(t, len(release.Values) == 1)
	expectedValues := helm.Values{
		"autoscaling": map[string]interface{}{
			"enabled": true,
		},
	}

	assert.DeepEqual(t, release.Values, expectedValues)

	subcomponent, err := builder.Build(WithProjectRoot(projectRoot), WithComponentPath("infra/prometheus/subcomponent"))
	assert.NilError(t, err)
	subcomponentManifests := subcomponent.Manifests
	assert.Assert(t, len(subcomponentManifests) == 1)
	deployment := subcomponent.Manifests[0]
	assert.Equal(t, deployment.GetAPIVersion(), "apps/v1")
	assert.Equal(t, deployment.GetKind(), "Deployment")
	assert.Equal(t, deployment.GetName(), "mysubcomponent")
	deploySpec := deployment.Object["spec"].(map[string]interface{})
	replicas := deploySpec["replicas"]
	if _, ok := replicas.(int64); ok {
		assert.Equal(t, replicas, int64(1))
	} else {
		assert.Equal(t, replicas, 1)
	}
	deployTemplate := deploySpec["template"].(map[string]interface{})
	deployTemplateSpec := deployTemplate["spec"].(map[string]interface{})
	deployContainers := deployTemplateSpec["containers"].([]interface{})
	assert.Equal(t, len(deployContainers), 1)
	deployContainer := deployContainers[0].(map[string]interface{})
	assert.Equal(t, deployContainer["name"], "subcomponent")
	assert.Equal(t, deployContainer["image"], "subcomponent:1.14.2")
	deployContainerPorts := deployContainer["ports"].([]interface{})
	deployContainerPort := deployContainerPorts[0].(map[string]interface{})
	containerPort := deployContainerPort["containerPort"]
	if _, ok := containerPort.(int64); ok {
		assert.Equal(t, containerPort, int64(80))
	} else {
		assert.Equal(t, containerPort, 80)
	}
}
