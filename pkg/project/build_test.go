package project

import (
	"os"
	"path"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"gotest.tools/v3/assert"
)

func TestManifestInstanceBuilder_Build(t *testing.T) {
	ctx := cuecontext.New()
	builder := NewComponentBuilder(ctx)
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	projectRoot := path.Join(cwd, "testdata", "simple")
	component, err := builder.Build(WithProjectRoot(projectRoot), WithComponentPath("./infra/prometheus"))
	assert.Equal(t, component.IntervalSeconds, 1)
	unstructureds := component.Manifests
	assert.NilError(t, err)
	assert.Assert(t, len(unstructureds) == 2)
	deployment := unstructureds[1]
	assert.Equal(t, deployment.GetAPIVersion(), "v1")
	assert.Equal(t, deployment.GetKind(), "Deployment")
	assert.Equal(t, deployment.GetName(), "mydeployment")
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
	assert.Equal(t, deployContainer["name"], "nginx")
	assert.Equal(t, deployContainer["image"], "nginx:1.14.2")
	deployContainerPorts := deployContainer["ports"].([]interface{})
	deployContainerPort := deployContainerPorts[0].(map[string]interface{})
	containerPort := deployContainerPort["containerPort"]
	if _, ok := containerPort.(int64); ok {
		assert.Equal(t, containerPort, int64(80))
	} else {
		assert.Equal(t, containerPort, 80)
	}
	namespace := unstructureds[0].Object
	assert.Equal(t, namespace["apiVersion"], "v1")
	assert.Equal(t, namespace["kind"], "Namespace")
	nsMetadata := namespace["metadata"].(map[string]interface{})
	assert.Equal(t, nsMetadata["name"], "mynamespace")

	_, err = builder.Build(WithProjectRoot(projectRoot), WithComponentPath("./infra/prometheus/nodeexporter"))
	assert.NilError(t, err)
}
