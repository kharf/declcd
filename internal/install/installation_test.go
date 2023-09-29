package install_test

import (
	"context"
	"testing"

	"github.com/kharf/declcd/internal/install"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/kube"
	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestAction_Install(t *testing.T) {
	env := projecttest.StartProjectEnv(t)
	defer env.Stop()
	ctx := context.Background()
	kubeClient, err := kube.NewClient(env.ControlPlane.Config)
	assert.NilError(t, err)
	action := install.NewAction(kubeClient)
	nsName := "declcd-system"
	err = action.Install(ctx, install.Namespace(nsName), install.Image("image"))
	assert.NilError(t, err)
	var ns v1.Namespace
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
	assert.NilError(t, err)
	var pvc v1.PersistentVolumeClaim
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "declcd", Namespace: nsName}, &pvc)
	assert.NilError(t, err)
	var dep appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "declcd-controller", Namespace: nsName}, &dep)
	assert.NilError(t, err)
}
