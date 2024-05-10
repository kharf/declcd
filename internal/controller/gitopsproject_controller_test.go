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
	"time"

	gitops "github.com/kharf/declcd/api/v1beta1"
	"github.com/kharf/declcd/internal/install"
	"github.com/kharf/declcd/pkg/project"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	GitOpsProjectName      = "test"
	GitOpsProjectNamespace = "default"

	duration                = time.Second * 30
	intervalInSeconds       = 5
	assertionInterval       = (intervalInSeconds + 1) * time.Second
	projectCreationTimeout  = time.Second * 20
	projectCreationInterval = 1
)

var _ = Describe("GitOpsProject controller", func() {
	When("Creating GitOpsProject", func() {
		When("The pull interval is less than 5 seconds", func() {
			It("Should not allow a pull interval less than 5 seconds", func() {
				err := project.Init("github.com/kharf/declcd/controller", env.TestProject, "1.0.0")
				Expect(err).NotTo(HaveOccurred())
				err = installAction.Install(
					env.Ctx,
					install.Namespace(GitOpsProjectNamespace),
					install.URL(env.TestProject),
					install.Branch("main"),
					install.Name(GitOpsProjectName),
					install.Interval(0),
					install.Token("abcd"),
				)
				Expect(err).To(HaveOccurred())
				Expect(
					err.Error(),
				).To(Equal("GitOpsProject.gitops.declcd.io \"" + GitOpsProjectName + "\" " +
					"is invalid: spec.pullIntervalSeconds: " +
					"Invalid value: 0: spec.pullIntervalSeconds in body should be greater than or equal to 5"))
			})
		})

		When("The pull interval is greater than or equal to 5 seconds", func() {
			AfterEach(func() {
				err := k8sClient.DeleteAllOf(
					env.Ctx,
					&gitops.GitOpsProject{},
					client.InNamespace(GitOpsProjectNamespace),
				)
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.DeleteAllOf(
					env.Ctx,
					&appsv1.Deployment{},
					client.InNamespace("podinfo"),
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It(
				"Should reconcile the declared cluster state with the current cluster state",
				func() {
					ctx := context.Background()
					err := project.Init(
						"github.com/kharf/declcd/controller",
						env.TestProject,
						"1.0.0",
					)
					Expect(err).NotTo(HaveOccurred())
					err = installAction.Install(
						env.Ctx,
						install.Namespace(GitOpsProjectNamespace),
						install.URL(env.TestProject),
						install.Branch("main"),
						install.Name(GitOpsProjectName),
						install.Interval(intervalInSeconds),
						install.Token("abcd"),
					)
					Expect(err).NotTo(HaveOccurred())

					Eventually(func(g Gomega) {
						var project gitops.GitOpsProject
						err := k8sClient.Get(
							ctx,
							types.NamespacedName{
								Name:      GitOpsProjectName,
								Namespace: GitOpsProjectNamespace,
							},
							&project,
						)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(project.Spec.PullIntervalSeconds).To(Equal(intervalInSeconds))
						suspend := false
						g.Expect(project.Spec.Suspend).To(Equal(&suspend))
						g.Expect(project.Spec.URL).To(Equal(env.TestProject))
					}, duration, assertionInterval).Should(Succeed())

					By(
						"Cloning a decl gitops repository, building manifests and applying them onto the cluster",
					)

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
								Name:      GitOpsProjectName,
								Namespace: GitOpsProjectNamespace,
							},
							&updatedGitOpsProject,
						)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(updatedGitOpsProject.Status.Revision.CommitHash).ToNot(BeEmpty())
						g.Expect(updatedGitOpsProject.Status.Revision.ReconcileTime.IsZero()).
							To(BeFalse())
						g.Expect(len(updatedGitOpsProject.Status.Conditions)).To(Equal(2))
					}, duration, assertionInterval).Should(Succeed())
				},
			)
		})

	})
})
