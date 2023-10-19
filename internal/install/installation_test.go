package install_test

import (
	"context"
	"testing"

	gitopsv1 "github.com/kharf/declcd/api/v1"
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
	err = action.Install(
		ctx,
		install.Namespace(nsName),
		install.Branch("main"),
		install.Interval(5),
		install.Stage("dev"),
		install.URL("url"),
	)
	assert.NilError(t, err)
	var ns v1.Namespace
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
	assert.NilError(t, err)
	var statefulSet appsv1.StatefulSet
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "gitops-controller", Namespace: nsName}, &statefulSet)
	assert.NilError(t, err)
	var project gitopsv1.GitOpsProject
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "dev", Namespace: nsName}, &project)
	assert.NilError(t, err)
}