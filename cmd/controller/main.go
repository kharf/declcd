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

package main

import (
	"flag"
	"os"

	"helm.sh/helm/v3/pkg/action"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/controller"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/garbage"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/secret"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gitopsv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := ctrlZap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&opts))
	ctrl.SetLogger(log)
	cfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "597c047a.declcd.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}
	componentBuilder := component.NewBuilder()
	projectManager := project.NewManager(componentBuilder, log)
	helmCfg := action.Configuration{}
	getter := kube.InMemoryRESTClientGetter{
		Cfg:        cfg,
		RestMapper: mgr.GetRESTMapper(),
	}
	voidLog := func(string, ...interface{}) {}
	err = helmCfg.Init(getter, "default", "secret", voidLog)
	if err != nil {
		setupLog.Error(err, "Unable to init helm")
		os.Exit(1)
	}
	chartReconciler := helm.ChartReconciler{
		Cfg: helmCfg,
		Log: log,
	}
	inventoryManager := inventory.Manager{
		Log:  log,
		Path: "/inventory",
	}
	namespace, err := os.ReadFile("/podinfo/namespace")
	if err != nil {
		setupLog.Error(err, "Unable to read current namespace")
		os.Exit(1)
	}
	kubeDynamicClient, err := kube.NewDynamicClient(cfg)
	if err = (&controller.GitOpsProjectReconciler{
		Reconciler: project.Reconciler{
			Log:               log,
			Client:            mgr.GetClient(),
			ComponentBuilder:  componentBuilder,
			RepositoryManager: project.NewRepositoryManager(log),
			ProjectManager:    projectManager,
			ChartReconciler:   chartReconciler,
			InventoryManager:  inventoryManager,
			GarbageCollector: garbage.Collector{
				Log:              log,
				InventoryManager: inventoryManager,
				HelmConfig:       helmCfg,
			},
			Decrypter: secret.NewDecrypter(string(namespace), kubeDynamicClient),
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "GitOpsProject")
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}
	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
