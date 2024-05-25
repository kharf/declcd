// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/internal/install"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/secret"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var Version string
var OS string
var Arch string

func main() {
	cfg, err := initCliConfig()
	if err != nil {
		fmt.Println(err)
		return
	}
	root, err := initCli(cfg)
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
	verifyCommandBuilder  VerifyCommandBuilder
	versionCommandBuilder VersionCommandBuilder
	installCommandBuilder InstallCommandBuilder
	encryptCommandBuilder EncryptCommandBuilder
}

func (builder RootCommandBuilder) Build() *cobra.Command {
	rootCmd := cobra.Command{
		Use:   "declcd",
		Short: "A GitOps Declarative Continuous Delivery toolkit",
	}
	rootCmd.AddCommand(builder.initCommandBuilder.Build())
	rootCmd.AddCommand(builder.verifyCommandBuilder.Build())
	rootCmd.AddCommand(builder.versionCommandBuilder.Build())
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
			return project.Init(args[0], cwd, Version)
		},
	}
	return cmd
}

type VerifyCommandBuilder struct{}

func (builder VerifyCommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a Declcd Repository in the current directory, whether it contains valid code and can be compiled",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			projectManager := project.NewManager(
				component.NewBuilder(),
				logr.Discard(),
				runtime.GOMAXPROCS(0),
			)
			_, err = projectManager.Load(cwd)
			return err
		},
	}
	return cmd
}

type VersionCommandBuilder struct{}

func (builder VersionCommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print declcd version",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			fmt.Printf("declcd v%s\n", Version)
			fmt.Printf("on %s_%s\n", OS, Arch)
			return nil
		},
	}
	return cmd
}

type InstallCommandBuilder struct{}

func (builder InstallCommandBuilder) Build() *cobra.Command {
	ctx := context.Background()
	var branch string
	var url string
	var name string
	var token string
	var interval int
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Declcd on a Kubernetes Cluster",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			kubeConfig, err := config.GetConfig()
			if err != nil {
				return err
			}
			client, err := kube.NewDynamicClient(kubeConfig)
			if err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			httpClient := http.DefaultClient
			action := install.NewAction(client, httpClient, wd)
			if err := action.Install(ctx,
				install.Namespace(project.ControllerNamespace),
				install.URL(url),
				install.Branch(branch),
				install.Name(name),
				install.Interval(interval),
				install.Token(token),
			); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().
		StringVarP(&branch, "branch", "b", "main", "Branch of a gitops repository containing project configuration")
	cmd.Flags().StringVarP(&url, "url", "u", "", "Url to a gitops repository")
	cmd.Flags().
		StringVarP(&name, "name", "", "", "Name of the GitOpsProject")
	cmd.Flags().StringVarP(&token, "token", "t", "", "Access token used for authentication")
	cmd.Flags().
		IntVarP(&interval, "interval", "i", 30, "Definition of how often Declcd will reconcile its cluster state. Value is defined in seconds")
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
	config.SetEnvPrefix("declcd")
	config.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	return config, nil
}

func initCli(cliConfig *viper.Viper) (*RootCommandBuilder, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	encryptCommand := EncryptCommandBuilder{
		secretEncrypter: secret.NewEncrypter(wd),
	}
	rootCmd := RootCommandBuilder{
		encryptCommandBuilder: encryptCommand,
	}
	return &rootCmd, nil
}
