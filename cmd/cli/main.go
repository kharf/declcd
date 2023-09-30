package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/kharf/declcd/internal/install"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	cfg, err := initCliConfig()
	if err != nil {
		fmt.Println(err)
		return
	}

	kubeConfig, err := config.GetConfig()
	if err != nil {
		fmt.Println(err)
		return
	}
	root, err := initCli(cfg, kubeConfig)
	if err != nil {
		fmt.Println(err)
		return
	}

	if err := root.Build().Execute(); err != nil {
		fmt.Println(err)
		return
	}
}

type RootCommandBuilder struct {
	installCommandBuilder InstallCommandBuilder
}

func (builder RootCommandBuilder) Build() *cobra.Command {
	rootCmd := cobra.Command{
		Use:   "decl",
		Short: "A GitOps Declarative Continuous Delivery toolkit",
	}

	installCmd := builder.installCommandBuilder.Build()
	rootCmd.AddCommand(installCmd)

	return &rootCmd
}

type InstallCommandBuilder struct {
	action install.Action
}

func (builder InstallCommandBuilder) Build() *cobra.Command {
	ctx := context.Background()
	var branch string
	var url string
	var stage string
	var interval int
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Declcd on a Kubernetes Cluster",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			if err := builder.action.Install(ctx,
				install.Namespace("declcd-system"),
				install.URL(url),
				install.Branch(branch),
				install.Stage(stage),
				install.Interval(interval),
			); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&branch, "branch", "b", "main", "The branch of the gitops repository holding the declcd configuration")
	cmd.Flags().StringVarP(&url, "url", "u", "", "The url to the gitops repository")
	cmd.Flags().StringVarP(&stage, "stage", "s", "", "The stage of the declcd configuration")
	cmd.Flags().IntVarP(&interval, "interval", "i", 30, "This defines how often declcd will reconcile the cluster state. The value is defined in seconds")
	return cmd
}

func initCliConfig() (*viper.Viper, error) {
	config := viper.New()
	config.SetEnvPrefix("decl")
	config.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := config.BindEnv("github.token"); err != nil {
		return nil, err
	}

	return config, nil
}

func initCli(cliConfig *viper.Viper, kubeConfig *rest.Config) (*RootCommandBuilder, error) {
	client, err := kube.NewClient(kubeConfig)
	if err != nil {
		return nil, err
	}
	installCmd := InstallCommandBuilder{
		action: install.NewAction(client),
	}
	rootCmd := RootCommandBuilder{installCmd}

	return &rootCmd, nil
}
