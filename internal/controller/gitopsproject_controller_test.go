package controller

import (
	"context"
	"time"

	gitopsv1 "github.com/kharf/declcd/api/v1"
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
				Eventually(func() string {
					if err := project.Init("github.com/kharf/declcd/controller", env.TestProject); err != nil {
						return err.Error()
					}
					if err := installAction.Install(
						env.Ctx,
						install.Namespace(GitOpsProjectNamespace),
						install.URL(env.TestProject),
						install.Branch("main"),
						install.Name(GitOpsProjectName),
						install.Interval(0),
						install.Token("abcd"),
					); err != nil {
						return err.Error()
					}
					return ""
				}, projectCreationTimeout, projectCreationInterval).Should(Equal("GitOpsProject.gitops.declcd.io \"" + GitOpsProjectName + "\" " +
					"is invalid: spec.pullIntervalSeconds: " +
					"Invalid value: 0: spec.pullIntervalSeconds in body should be greater than or equal to 5",
				))
			})
		})

		When("The pull interval is greater than or equal to 5 seconds", func() {
			AfterEach(func() {
				err := k8sClient.DeleteAllOf(
					env.Ctx,
					&gitopsv1.GitOpsProject{},
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
					Eventually(func() error {
						if err := project.Init("github.com/kharf/declcd/controller", env.TestProject); err != nil {
							return err
						}
						return installAction.Install(
							env.Ctx,
							install.Namespace(GitOpsProjectNamespace),
							install.URL(env.TestProject),
							install.Branch("main"),
							install.Name(GitOpsProjectName),
							install.Interval(intervalInSeconds),
							install.Token("abcd"),
						)
					}, projectCreationTimeout, projectCreationInterval).Should(BeNil())
					Eventually(func(g Gomega) {
						var project gitopsv1.GitOpsProject
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

					Eventually(func() (int, error) {
						var updatedGitOpsProject gitopsv1.GitOpsProject
						if err := k8sClient.Get(ctx, types.NamespacedName{Name: GitOpsProjectName, Namespace: GitOpsProjectNamespace}, &updatedGitOpsProject); err != nil {
							return 0, err
						}
						return len(updatedGitOpsProject.Status.Conditions), nil
					}, duration, assertionInterval).Should(Equal(2))
				},
			)
		})

	})
})
