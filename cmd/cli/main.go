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

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var Version string
var OS string
var Arch string

func main() {
	root := RootCommandBuilder{}
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
	return &rootCmd
}

type InitCommandBuilder struct{}

func (builder InitCommandBuilder) Build() *cobra.Command {
	var shard string
	var isSecondary bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init a Declcd Project in the current directory",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return project.Init(
				args[0],
				shard,
				isSecondary,
				cwd,
				Version,
			)
		},
	}
	cmd.Flags().
		StringVar(&shard, "shard", "primary", "Instance of the Declcd Project")
	cmd.Flags().
		BoolVar(&isSecondary, "secondary", false, "Indicates a secondary Declcd instance")
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
			fmt.Printf("declcd v%s\non %s_%s\n", Version, OS, Arch)
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
	var shard string
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
			action := project.NewInstallAction(client, httpClient, wd)
			if err := action.Install(ctx,
				project.InstallOptions{
					Url:      url,
					Branch:   branch,
					Name:     name,
					Interval: interval,
					Token:    token,
					Shard:    shard,
				},
			); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().
		StringVarP(&branch, "branch", "b", "main", "Branch of the GitOps Repository containing project configuration")
	cmd.Flags().StringVarP(&url, "url", "u", "", "Url to the GitOps repository")
	cmd.Flags().
		StringVar(&name, "name", "", "Name of the GitOps Project")
	cmd.Flags().StringVarP(&token, "token", "t", "", "Access token used for authentication")
	cmd.Flags().
		IntVarP(&interval, "interval", "i", 30, "Definition of how often Declcd will reconcile its cluster state. Value is defined in seconds")
	cmd.Flags().
		StringVar(&shard, "shard", "primary", "Instance associated with the Declcd Project")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}
