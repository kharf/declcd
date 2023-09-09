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
	"path/filepath"

	"cuelang.org/go/cue/cuecontext"
	"go.uber.org/zap"
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

	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/controller"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(gitopsv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
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
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	log := ctrlZap.New(ctrlZap.UseFlagOptions(&opts))
	ctrl.SetLogger(log)

	cfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
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
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	gitOpsRepositoryDir := filepath.Join(os.TempDir(), "decl")
	fs := os.DirFS(gitOpsRepositoryDir)
	ctx := cuecontext.New()
	// TODO: replace with logr
	ctrlZapConfig := zap.NewDevelopmentConfig()
	ctrlZapConfig.OutputPaths = []string{"stdout"}
	logger, err := ctrlZapConfig.Build()
	if err != nil {
		setupLog.Error(err, "unable to build logger")
		os.Exit(1)
	}
	defer logger.Sync()
	sugarredLogger := logger.Sugar()
	projectManager := project.NewProjectManager(project.FileSystem{FS: fs, Root: gitOpsRepositoryDir}, sugarredLogger)
	helmCfg := action.Configuration{}
	getter := kube.InMemoryRESTClientGetter{
		Cfg:        cfg,
		RestMapper: mgr.GetRESTMapper(),
	}
	err = helmCfg.Init(getter, "default", "secret", sugarredLogger.Infof)
	chartReconciler := helm.ChartReconciler{Cfg: helmCfg}
	if err = (&controller.GitOpsProjectReconciler{
		Reconciler: project.Reconciler{
			Client:            mgr.GetClient(),
			CueContext:        ctx,
			RepositoryManager: project.NewRepositoryManager(log),
			ProjectManager:    projectManager,
			ChartReconciler:   chartReconciler,
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitOpsProject")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
