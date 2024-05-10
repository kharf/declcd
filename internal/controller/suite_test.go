/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	goRuntime "runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/install"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/vcs"

	_ "github.com/kharf/declcd/test/workingdir"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient     client.Client
	test          *testing.T
	env           projecttest.Environment
	httpClient    *http.Client
	installAction install.Action
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	test = t

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	env = projecttest.StartProjectEnv(test,
		projecttest.WithKubernetes(
			kubetest.WithHelm(true, false, false),
			kubetest.WithDecryptionKeyCreated(),
			kubetest.WithVCSSSHKeyCreated(),
		),
	)
	var server *httptest.Server
	server, httpClient = gittest.MockGitProvider(test, vcs.GitHub)
	defer server.Close()
	installAction = install.NewAction(
		env.DynamicTestKubeClient,
		httpClient,
		env.TestProject,
	)
	logf.SetLogger(env.Log)
	var err error
	k8sClient, err = client.New(env.ControlPlane.Config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
	chartReconciler := helm.ChartReconciler{
		KubeConfig:            env.ControlPlane.Config,
		Client:                env.DynamicTestKubeClient,
		FieldManager:          project.ControllerName,
		InventoryManager:      env.InventoryManager,
		InsecureSkipTLSverify: true,
		Log:                   env.Log,
	}
	reconciliationHisto := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "declcd",
		Name:      "reconciliation_duration_seconds",
		Help:      "Duration of a GitOps Project reconciliation",
	}, []string{"project", "url"})
	reconciler := project.Reconciler{
		Client:            env.ControllerManager.GetClient(),
		DynamicClient:     env.DynamicTestKubeClient,
		ComponentBuilder:  component.NewBuilder(),
		RepositoryManager: env.RepositoryManager,
		ProjectManager:    env.ProjectManager,
		ChartReconciler:   chartReconciler,
		InventoryManager:  env.InventoryManager,
		Log:               env.Log,
		GarbageCollector:  env.GarbageCollector,
		Decrypter:         env.SecretManager.Decrypter,
		FieldManager:      project.ControllerName,
		WorkerPoolSize:    goRuntime.GOMAXPROCS(0),
	}
	err = (&GitOpsProjectReconciler{
		Reconciler:              reconciler,
		ReconciliationHistogram: reconciliationHisto,
	}).SetupWithManager(env.ControllerManager)
	Expect(err).ToNot(HaveOccurred())
	go func() {
		defer GinkgoRecover()
		err = env.ControllerManager.Start(env.Ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	env.Stop()
})
