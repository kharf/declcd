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
	_ "go.uber.org/automaxprocs"
	"net/http"
	_ "net/http/pprof"
	"os"

	"cuelang.org/go/pkg/strings"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	_ "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"
	"github.com/prometheus/client_golang/prometheus"

	"go.uber.org/zap/zapcore"
	helmKube "helm.sh/helm/v3/pkg/kube"

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

	goRuntime "runtime"

	gitops "github.com/kharf/declcd/api/v1beta1"
	"github.com/kharf/declcd/internal/controller"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/garbage"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/secret"
	"github.com/kharf/declcd/pkg/vcs"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	Version  string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gitops.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var logLevel int
	var inventoryPath string
	var namespacePodinfoPath string
	var insecureSkipTLSverify bool
	flag.StringVar(
		&metricsAddr,
		"metrics-bind-address",
		":8080",
		"The address the metric endpoint binds to.",
	)
	flag.StringVar(
		&probeAddr,
		"health-probe-bind-address",
		":8081",
		"The address the probe endpoint binds to.",
	)
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&logLevel, "log-level", 0, "The verbosity level. Higher means chattier.")
	flag.StringVar(
		&inventoryPath,
		"inventory-path",
		"/inventory",
		"The directory the inventory uses to store cluster state items.",
	)
	flag.StringVar(
		&namespacePodinfoPath,
		"namespace-podinfo-path",
		"/podinfo/namespace",
		"The file which holds the controller namespace.",
	)
	flag.BoolVar(
		&insecureSkipTLSverify,
		"insecure-skip-tls-verify",
		false,
		"InsecureSkipVerify controls whether the Helm client verifies the server's certificate chain and host name.	",
	)
	flag.Parse()

	opts := ctrlZap.Options{
		Development: false,
		Level:       zapcore.Level(logLevel * -1),
	}
	opts.BindFlags(flag.CommandLine)
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&opts))
	ctrl.SetLogger(log)

	cfg := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
			ExtraHandlers: map[string]http.Handler{
				"/debug/pprof/": http.DefaultServeMux,
			},
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

	if err = os.Setenv("CUE_EXPERIMENT", "modules"); err != nil {
		setupLog.Error(err, "Unable to set CUE_EXPERIMENT environment variable")
		os.Exit(1)
	}

	if err = os.Setenv("CUE_REGISTRY", "ghcr.io/kharf"); err != nil {
		setupLog.Error(err, "Unable to set CUE_REGISTRY environment variable")
		os.Exit(1)
	}

	componentBuilder := component.NewBuilder()

	maxProcs := goRuntime.GOMAXPROCS(0)
	log.V(1).Info("GOMAXPROCS", "value", maxProcs)

	projectManager := project.NewManager(componentBuilder, log, maxProcs)

	//TODO: downward api read controller from file
	helmKube.ManagedFieldsManager = project.ControllerName

	kubeDynamicClient, err := kube.NewDynamicClient(cfg)
	if err != nil {
		setupLog.Error(err, "Unable to setup Kubernetes client")
		os.Exit(1)
	}

	inventoryManager := &inventory.Manager{
		Log:  log,
		Path: inventoryPath,
	}

	chartReconciler := helm.ChartReconciler{
		KubeConfig:            cfg,
		Client:                kubeDynamicClient,
		FieldManager:          project.ControllerName,
		InventoryManager:      inventoryManager,
		InsecureSkipTLSverify: insecureSkipTLSverify,
		Log:                   log,
	}

	namespaceBytes, err := os.ReadFile(namespacePodinfoPath)
	if err != nil {
		setupLog.Error(err, "Unable to read current namespace")
		os.Exit(1)
	}

	namespace := strings.TrimSpace(string(namespaceBytes))

	reconciliationHisto := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "declcd",
		Name:      "reconciliation_duration_seconds",
		Help:      "Duration of a GitOps Project reconciliation",
	}, []string{"project", "url"})
	metrics.Registry.MustRegister(reconciliationHisto)

	if err := (&controller.GitOpsProjectReconciler{
		Reconciler: project.Reconciler{
			Log:               log,
			Client:            mgr.GetClient(),
			DynamicClient:     kubeDynamicClient,
			ComponentBuilder:  componentBuilder,
			RepositoryManager: vcs.NewRepositoryManager(namespace, kubeDynamicClient, log),
			ProjectManager:    projectManager,
			ChartReconciler:   chartReconciler,
			InventoryManager:  inventoryManager,
			GarbageCollector: garbage.Collector{
				Log:              log,
				Client:           kubeDynamicClient,
				KubeConfig:       cfg,
				InventoryManager: inventoryManager,
				WorkerPoolSize:   maxProcs,
			},
			Decrypter:      secret.NewDecrypter(namespace, kubeDynamicClient, maxProcs),
			FieldManager:   project.ControllerName,
			WorkerPoolSize: maxProcs,
		},
		ReconciliationHistogram: reconciliationHisto,
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
