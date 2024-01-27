package install_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/install"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/secret"
	"github.com/kharf/declcd/pkg/vcs"
	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestAction_Install(t *testing.T) {
	server, client := gittest.MockGitProvider(t, vcs.GitHub)
	defer server.Close()
	env := projecttest.StartProjectEnv(t)
	defer env.Stop()
	ctx := context.Background()
	kubeClient, err := kube.NewDynamicClient(env.ControlPlane.Config)
	assert.NilError(t, err)
	action := install.NewAction(kubeClient, client, env.TestProject)
	nsName := install.ControllerNamespace
	err = action.Install(
		ctx,
		install.Namespace(nsName),
		install.Branch("main"),
		install.Interval(5),
		install.Stage("dev"),
		install.URL("git@github.com:kharf/declcd.git"),
		install.Token("aaaa"),
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
	var decKey v1.Secret
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: secret.K8sSecretName, Namespace: nsName}, &decKey)
	assert.NilError(t, err)
	_, err = os.Open(filepath.Join(env.TestProject, "secrets/recipients.cue"))
	assert.NilError(t, err)
	var vcsKey v1.Secret
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: vcs.K8sSecretName, Namespace: nsName}, &vcsKey)
	assert.NilError(t, err)
}
