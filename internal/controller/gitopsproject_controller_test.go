package controller

import (
	"context"
	"time"

	gitopsv1 "github.com/kharf/declcd/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("GitOpsProject controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		GitOpsProjectName      = "test"
		GitOpsProjectNamespace = "default"

		duration                = time.Second * 15
		intervalInSeconds       = 5
		assertionInterval       = (intervalInSeconds + 1) * time.Second
		projectCreationTimeout  = time.Second * 20
		projectCreationInterval = 1
	)

	When("Creating GitOpsProject", func() {
		var (
			gitOpsProject gitopsv1.GitOpsProject
			suspend       bool
		)

		BeforeEach(func() {
			suspend := false
			gitOpsProject = gitopsv1.GitOpsProject{
				TypeMeta: metav1.TypeMeta{
					APIVersion: gitopsv1.GroupVersion.String(),
					Kind:       "GitOpsProject",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      GitOpsProjectName,
					Namespace: GitOpsProjectNamespace,
				},
				Spec: gitopsv1.GitOpsProjectSpec{
					URL:                 env.TestProject,
					PullIntervalSeconds: intervalInSeconds,
					Suspend:             &suspend,
					Branch:              "main",
					Stage:               "dev",
				},
			}
		})

		When("The pull interval is less than 5 seconds", func() {
			It("Should not allow a pull interval less than 5 seconds", func() {
				ctx := context.Background()
				gitOpsProject.Spec.PullIntervalSeconds = 0

				Eventually(func() string {
					if err := k8sClient.Create(ctx, &gitOpsProject); err != nil {
						return err.Error()
					}
					return ""
				}, projectCreationTimeout, projectCreationInterval).Should(Equal("GitOpsProject.gitops.declcd.io \"test\" " +
					"is invalid: spec.pullIntervalSeconds: " +
					"Invalid value: 0: spec.pullIntervalSeconds in body should be greater than or equal to 5",
				))
			})
		})

		When("The pull interval is greater than or equal to 5 seconds", func() {
			AfterEach(func() {
				err := k8sClient.DeleteAllOf(env.Ctx, &gitopsv1.GitOpsProject{}, client.InNamespace(GitOpsProjectNamespace))
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.DeleteAllOf(env.Ctx, &appsv1.Deployment{}, client.InNamespace("podinfo"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should reconcile the declared cluster state with the current cluster state", func() {
				ctx := context.Background()
				Eventually(func() error {
					return k8sClient.Create(ctx, &gitOpsProject)
				}, projectCreationTimeout, projectCreationInterval).Should(BeNil())
				Expect(gitOpsProject.Spec.PullIntervalSeconds).To(Equal(intervalInSeconds))
				Expect(gitOpsProject.Spec.Suspend).To(Equal(&suspend))
				Expect(gitOpsProject.Spec.URL).To(Equal(env.TestProject))

				By("Cloning a decl gitops repository, building manifests and applying them onto the cluster")
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
			})
		})

	})
})
