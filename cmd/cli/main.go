package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/kharf/declcd/internal/install"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/secret"
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
	initCommandBuilder    InitCommandBuilder
	installCommandBuilder InstallCommandBuilder
	encryptCommandBuilder EncryptCommandBuilder
}

func (builder RootCommandBuilder) Build() *cobra.Command {
	rootCmd := cobra.Command{
		Use:   "decl",
		Short: "A GitOps Declarative Continuous Delivery toolkit",
	}
	rootCmd.AddCommand(builder.initCommandBuilder.Build())
	rootCmd.AddCommand(builder.installCommandBuilder.Build())
	rootCmd.AddCommand(builder.encryptCommandBuilder.Build())
	return &rootCmd
}

type InitCommandBuilder struct{}

func (builder InitCommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init a Declcd Repository in the current directory",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return project.Init(args[0], cwd)
		},
	}
	return cmd
}

type InstallCommandBuilder struct {
	action install.Action
}

func (builder InstallCommandBuilder) Build() *cobra.Command {
	ctx := context.Background()
	var branch string
	var url string
	var stage string
	var token string
	var interval int
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Declcd on a Kubernetes Cluster",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			if err := builder.action.Install(ctx,
				install.Namespace(install.ControllerNamespace),
				install.URL(url),
				install.Branch(branch),
				install.Stage(stage),
				install.Interval(interval),
				install.Token(token),
			); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().
		StringVarP(&branch, "branch", "b", "main", "The branch of the gitops repository containing the project configuration")
	cmd.Flags().StringVarP(&url, "url", "u", "", "The url to the gitops repository")
	cmd.Flags().StringVarP(&stage, "stage", "s", "", "The stage of the declcd configuration")
	cmd.Flags().StringVarP(&token, "token", "t", "", "The access token used for authentication")
	cmd.Flags().
		IntVarP(&interval, "interval", "i", 30, "This defines how often declcd will reconcile the cluster state. The value is defined in seconds")
	return cmd
}

type EncryptCommandBuilder struct {
	secretEncrypter secret.Encrypter
}

func (builder EncryptCommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "encrypt",
		Short: "Encrypt Secrets inside the GitOps Repository",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			if err := builder.secretEncrypter.EncryptPackage(args[0]); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

func initCliConfig() (*viper.Viper, error) {
	config := viper.New()
	config.SetEnvPrefix("decl")
	config.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	return config, nil
}

func initCli(cliConfig *viper.Viper, kubeConfig *rest.Config) (*RootCommandBuilder, error) {
	client, err := kube.NewDynamicClient(kubeConfig)
	if err != nil {
		return nil, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	httpClient := http.DefaultClient
	installCmd := InstallCommandBuilder{
		action: install.NewAction(client, httpClient, wd),
	}
	encryptCommand := EncryptCommandBuilder{
		secretEncrypter: secret.NewEncrypter(wd),
	}
	rootCmd := RootCommandBuilder{
		installCommandBuilder: installCmd,
		encryptCommandBuilder: encryptCommand,
	}
	return &rootCmd, nil
}
