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

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gitops "github.com/kharf/declcd/api/v1beta1"
	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/helmtest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/vcs"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	gitOpsProjectNamespace = "declcd-system"

	duration                = time.Second * 30
	intervalInSeconds       = 5
	assertionInterval       = (intervalInSeconds + 1) * time.Second
	projectCreationTimeout  = time.Second * 20
	projectCreationInterval = 1
)

var _ = Describe("GitOpsProject controller", Ordered, func() {

	BeforeAll(func() {
		err := os.Mkdir("/podinfo", 0700)
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile("/podinfo/name", []byte("project-controller-primary"), 0600)
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile("/podinfo/namespace", []byte(project.ControllerNamespace), 0600)
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile("/podinfo/shard", []byte("primary"), 0600)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		err := os.RemoveAll("/podinfo")
		Expect(err).NotTo(HaveOccurred())
	})

	When("Creating a GitOpsProject", func() {

		var (
			env       projecttest.Environment
			k8sClient client.Client
		)

		BeforeEach(func() {
			env = projecttest.StartProjectEnv(test,
				projecttest.WithKubernetes(),
				projecttest.WithProjectSource("simple"),
			)
			var err error

			testProject := env.Projects[0]
			err = helmtest.ReplaceTemplate(
				helmtest.Template{
					TestProjectPath:         testProject.TargetPath,
					RelativeReleaseFilePath: "infra/prometheus/releases.cue",
					Name:                    "test",
					RepoURL:                 helmEnvironment.ChartServer.URL(),
				},
				testProject.GitRepository,
			)
			Expect(err).NotTo(HaveOccurred())

			k8sClient, err = client.New(
				env.ControlPlane.Config,
				client.Options{Scheme: scheme.Scheme},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient).NotTo(BeNil())
		})

		AfterEach(func() {
			env.Stop()
			metrics.Registry = prometheus.NewRegistry()
		})

		When("The pull interval is less than 5 seconds", func() {

			It("Should not allow a pull interval less than 5 seconds", func() {
				var gitOpsProjectName = "test"
				testProject := env.Projects[0]
				err := project.Init(
					"github.com/kharf/declcd/controller",
					"primary",
					false,
					testProject.TargetPath,
					"1.0.0",
				)
				Expect(err).NotTo(HaveOccurred())

				gitServer, httpClient := gittest.MockGitProvider(
					test,
					vcs.GitHub,
					fmt.Sprintf("declcd-%s", gitOpsProjectName),
				)
				defer gitServer.Close()

				installAction := project.NewInstallAction(
					env.DynamicTestKubeClient.DynamicClient(),
					httpClient,
					testProject.TargetPath,
				)

				err = installAction.Install(
					env.Ctx,
					project.InstallOptions{
						Url:      testProject.TargetPath,
						Branch:   "main",
						Name:     gitOpsProjectName,
						Shard:    "primary",
						Interval: 0,
						Token:    "abcd",
					},
				)
				Expect(err).To(HaveOccurred())
				Expect(
					err.Error(),
				).To(Equal("GitOpsProject.gitops.declcd.io \"" + "test" + "\" " +
					"is invalid: spec.pullIntervalSeconds: " +
					"Invalid value: 0: spec.pullIntervalSeconds in body should be greater than or equal to 5"))
			})
		})

		When("The pull interval is greater than or equal to 5 seconds", func() {
			var gitOpsProjectName = "test"

			It(
				"Should reconcile the declared cluster state with the current cluster state",
				func() {
					testProject := env.Projects[0]
					ctx := context.Background()
					err := project.Init(
						"github.com/kharf/declcd/controller",
						"primary",
						false,
						testProject.TargetPath,
						"1.0.0",
					)
					Expect(err).NotTo(HaveOccurred())
					gitServer, httpClient := gittest.MockGitProvider(
						test,
						vcs.GitHub,
						fmt.Sprintf("declcd-%s", gitOpsProjectName),
					)
					defer gitServer.Close()

					installAction := project.NewInstallAction(
						env.DynamicTestKubeClient.DynamicClient(),
						httpClient,
						testProject.TargetPath,
					)

					err = installAction.Install(
						env.Ctx,
						project.InstallOptions{
							Url:      testProject.TargetPath,
							Branch:   "main",
							Name:     gitOpsProjectName,
							Shard:    "primary",
							Interval: intervalInSeconds,
							Token:    "abcd",
						},
					)
					Expect(err).NotTo(HaveOccurred())

					mgr, err := Setup(
						env.ControlPlane.Config,
						InsecureSkipTLSverify(true),
						MetricsAddr("0"),
					)
					Expect(err).NotTo(HaveOccurred())

					go func() {
						defer GinkgoRecover()
						err := mgr.Start(env.Ctx)
						Expect(err).ToNot(HaveOccurred(), "failed to run manager")
					}()

					Eventually(func(g Gomega) {
						var project gitops.GitOpsProject
						err := k8sClient.Get(
							ctx,
							types.NamespacedName{
								Name:      gitOpsProjectName,
								Namespace: gitOpsProjectNamespace,
							},
							&project,
						)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(project.Spec.PullIntervalSeconds).To(Equal(intervalInSeconds))
						suspend := false
						g.Expect(project.Spec.Suspend).To(Equal(&suspend))
						g.Expect(project.Spec.URL).To(Equal(testProject.TargetPath))
					}, duration, assertionInterval).Should(Succeed())

					Eventually(func() (string, error) {
						var namespace corev1.Namespace
						if err := k8sClient.Get(ctx, types.NamespacedName{Name: "prometheus", Namespace: ""}, &namespace); err != nil {
							return "", err
						}
						return namespace.GetName(), nil
					}, duration, assertionInterval).Should(Equal("prometheus"))

					Eventually(func() (string, error) {
						var deployment appsv1.Deployment
						if err := k8sClient.Get(ctx, types.NamespacedName{Name: "mysubcomponent", Namespace: "prometheus"}, &deployment); err != nil {
							return "", err
						}
						return deployment.GetName(), nil
					}, duration, assertionInterval).Should(Equal("mysubcomponent"))

					Eventually(func(g Gomega) {
						var updatedGitOpsProject gitops.GitOpsProject
						err := k8sClient.Get(
							ctx,
							types.NamespacedName{
								Name:      gitOpsProjectName,
								Namespace: gitOpsProjectNamespace,
							},
							&updatedGitOpsProject,
						)
						g.Expect(err).ToNot(HaveOccurred())
					}, duration, assertionInterval).Should(Succeed())
				},
			)
		})
	})

	When("Creating multiple GitOpsProjects", func() {

		var (
			env           projecttest.Environment
			k8sClient     client.Client
			installAction project.InstallAction
		)

		BeforeAll(func() {
			var err error

			env = projecttest.StartProjectEnv(test,
				projecttest.WithKubernetes(),
				projecttest.WithProjectSource("simple"),
				projecttest.WithProjectSource("mini"),
			)

			k8sClient, err = client.New(
				env.ControlPlane.Config,
				client.Options{Scheme: scheme.Scheme},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient).NotTo(BeNil())

			for _, testProject := range env.Projects {
				releaseFilePath := "infra/prometheus/releases.cue"
				_, err := os.Stat(
					filepath.Join(testProject.TargetPath, "infra", "monitoring", "releases.cue"),
				)
				if err == nil {
					releaseFilePath = "infra/monitoring/releases.cue"
				}

				err = helmtest.ReplaceTemplate(
					helmtest.Template{
						TestProjectPath:         testProject.TargetPath,
						RelativeReleaseFilePath: releaseFilePath,
						Name:                    testProject.Name,
						RepoURL:                 helmEnvironment.ChartServer.URL(),
					},
					testProject.GitRepository,
				)
				Expect(err).NotTo(HaveOccurred())

				gitServer, httpClient := gittest.MockGitProvider(
					test,
					vcs.GitHub,
					fmt.Sprintf("declcd-%s", testProject.Name),
				)
				defer gitServer.Close()

				installAction = project.NewInstallAction(
					env.DynamicTestKubeClient.DynamicClient(),
					httpClient,
					testProject.TargetPath,
				)

				err = project.Init(
					"github.com/kharf/declcd/controller",
					"primary",
					false,
					testProject.TargetPath,
					"1.0.0",
				)
				Expect(err).NotTo(HaveOccurred())

				err = installAction.Install(
					env.Ctx,
					project.InstallOptions{
						Url:      testProject.TargetPath,
						Branch:   "main",
						Name:     testProject.Name,
						Shard:    "primary",
						Interval: intervalInSeconds,
						Token:    "abcd",
					},
				)
				Expect(err).NotTo(HaveOccurred())
			}

			mgr, err := Setup(
				env.ControlPlane.Config,
				InsecureSkipTLSverify(true),
			)
			Expect(err).NotTo(HaveOccurred())

			go func() {
				defer GinkgoRecover()
				err := mgr.Start(env.Ctx)
				Expect(err).ToNot(HaveOccurred(), "failed to run manager")
			}()
		})

		AfterAll(func() {
			env.Stop()
		})

		It(
			"Should reconcile the declared cluster state with the current cluster state",
			func() {
				ctx := context.Background()

				simpleProject := env.Projects[0]
				miniProject := env.Projects[1]

				Eventually(func(g Gomega) {
					var project gitops.GitOpsProject
					err := k8sClient.Get(
						ctx,
						types.NamespacedName{
							Name:      simpleProject.Name,
							Namespace: gitOpsProjectNamespace,
						},
						&project,
					)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(project.Spec.PullIntervalSeconds).To(Equal(intervalInSeconds))
					suspend := false
					g.Expect(project.Spec.Suspend).To(Equal(&suspend))
					g.Expect(project.Spec.URL).To(Equal(simpleProject.TargetPath))
				}, duration, assertionInterval).Should(Succeed())

				Eventually(func(g Gomega) {
					var project gitops.GitOpsProject
					err := k8sClient.Get(
						ctx,
						types.NamespacedName{
							Name:      miniProject.Name,
							Namespace: gitOpsProjectNamespace,
						},
						&project,
					)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(project.Spec.PullIntervalSeconds).To(Equal(intervalInSeconds))
					suspend := false
					g.Expect(project.Spec.Suspend).To(Equal(&suspend))
					g.Expect(project.Spec.URL).To(Equal(miniProject.TargetPath))
				}, duration, assertionInterval).Should(Succeed())

				Eventually(func(g Gomega) {
					var updatedGitOpsProject gitops.GitOpsProject
					err := k8sClient.Get(
						ctx,
						types.NamespacedName{
							Name:      simpleProject.Name,
							Namespace: gitOpsProjectNamespace,
						},
						&updatedGitOpsProject,
					)
					g.Expect(err).ToNot(HaveOccurred())
				}, duration, assertionInterval).Should(Succeed())

				Eventually(func(g Gomega) {
					var updatedGitOpsProject gitops.GitOpsProject
					err := k8sClient.Get(
						ctx,
						types.NamespacedName{
							Name:      miniProject.Name,
							Namespace: gitOpsProjectNamespace,
						},
						&updatedGitOpsProject,
					)
					g.Expect(err).ToNot(HaveOccurred())
				}, duration, assertionInterval).Should(Succeed())

				Eventually(func() (string, error) {
					var namespace corev1.Namespace
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: "prometheus", Namespace: ""}, &namespace); err != nil {
						return "", err
					}
					return namespace.GetName(), nil
				}, duration, assertionInterval).Should(Equal("prometheus"))

				Eventually(func() (string, error) {
					var deployment appsv1.Deployment
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: "mysubcomponent", Namespace: "prometheus"}, &deployment); err != nil {
						return "", err
					}
					return deployment.GetName(), nil
				}, duration, assertionInterval).Should(Equal("mysubcomponent"))

				Eventually(func() (string, error) {
					var namespace corev1.Namespace
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: "monitoring", Namespace: ""}, &namespace); err != nil {
						return "", err
					}
					return namespace.GetName(), nil
				}, duration, assertionInterval).Should(Equal("monitoring"))
			},
		)
	})
})
