package core

import (
	"os"
	"path"
	"testing"
	"testing/fstest"

	"cuelang.org/go/cue/cuecontext"
	"gotest.tools/v3/assert"
)

func TestContentEntryBuilder_Build(t *testing.T) {
	ctx := cuecontext.New()
	builder := NewContentEntryBuilder(ctx)
	result, err := builder.Build(`
		entry: app: {
		 intervalSeconds: 1
		}
	`,
	)
	assert.NilError(t, err)
	assert.Equal(t, result.Name, "app")
	assert.Equal(t, result.IntervalSeconds, 1)

	result, err = builder.Build(`
		entry: infrastructure: {
		 intervalSeconds: 60
		}
	`,
	)
	assert.NilError(t, err)
	assert.Equal(t, result.Name, "infrastructure")
	assert.Equal(t, result.IntervalSeconds, 60)
}

func TestContentEntryBuilder_Build_Schema(t *testing.T) {
	ctx := cuecontext.New()
	builder := NewContentEntryBuilder(ctx)
	_, err := builder.Build(`
		entry: app: {
		 intervalSeconds: "60"
		}
	`,
	)
	assert.Error(t, err, "entry.app.intervalSeconds: 2 errors in empty disjunction: (and 2 more errors)")
}

func TestFileEntryBuilder_Build(t *testing.T) {
	ctx := cuecontext.New()
	fs := fstest.MapFS{
		"entry.cue": {
			Data: []byte(`
		entry: app: {
		 intervalSeconds: 60
		}
	`),
		},
	}
	builder := NewFileEntryBuilder(ctx, fs, NewContentEntryBuilder(ctx))
	result, err := builder.Build("entry.cue")
	assert.NilError(t, err)
	assert.Equal(t, result.Name, "app")
	assert.Equal(t, result.IntervalSeconds, 60)
}

func TestManifestInstanceBuilder_Build(t *testing.T) {
	ctx := cuecontext.New()
	builder := NewComponentManifestBuilder(ctx)
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	projectRoot := path.Join(cwd, "testdata", "mib")
	unstructureds, err := builder.Build(WithProjectRoot(projectRoot), WithComponent("prometheus", "./infra/prometheus"))
	assert.NilError(t, err)
	assert.Assert(t, len(unstructureds) == 2)
	deployment := unstructureds[0]
	assert.Equal(t, deployment.GetAPIVersion(), "v1")
	assert.Equal(t, deployment.GetKind(), "Deployment")
	assert.Equal(t, deployment.GetName(), "mydeployment")
	deploySpec := deployment.Object["spec"].(map[string]interface{})
	assert.Equal(t, deploySpec["replicas"], 1)
	deployTemplate := deploySpec["template"].(map[string]interface{})
	deployTemplateSpec := deployTemplate["spec"].(map[string]interface{})
	deployContainers := deployTemplateSpec["containers"].([]interface{})
	assert.Equal(t, len(deployContainers), 1)
	deployContainer := deployContainers[0].(map[string]interface{})
	assert.Equal(t, deployContainer["name"], "nginx")
	assert.Equal(t, deployContainer["image"], "nginx:1.14.2")
	deployContainerPorts := deployContainer["ports"].([]interface{})
	deployContainerPort := deployContainerPorts[0].(map[string]interface{})
	assert.Equal(t, deployContainerPort["containerPort"], 80)
	namespace := unstructureds[1].Object
	assert.Equal(t, namespace["apiVersion"], "v1")
	assert.Equal(t, namespace["kind"], "Namespace")
	nsMetadata := namespace["metadata"].(map[string]interface{})
	assert.Equal(t, nsMetadata["name"], "mynamespace")

	_, err = builder.Build(WithProjectRoot(projectRoot), WithComponent("nodeexporter", "./infra/prometheus/nodeexporter"))
	assert.NilError(t, err)
}
