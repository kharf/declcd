package controller

import (
	"context"
	"path/filepath"
	"time"

	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/projecttest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("GitOpsProject controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		GitOpsProjectName      = "test"
		GitOpsProjectNamespace = "default"

		timeout           = time.Second * 15
		duration          = time.Second * 15
		intervalInSeconds = 5
		assertionInterval = (intervalInSeconds + 1) * time.Second
	)

	When("Creating GitOpsProject", func() {
		var (
			gitOpsProject   gitopsv1.GitOpsProject
			suspend         bool
			localRepository *projecttest.LocalGitRepository
		)

		BeforeEach(func() {
			var err error
			localRepository, err = projecttest.OpenGitRepository(filepath.Join(TestRoot, "controllertest"))
			Expect(err).NotTo(HaveOccurred())
			suspend := false
			gitOpsProject = gitopsv1.GitOpsProject{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "gitops.declcd.io/v1",
					Kind:       "GitOpsProject",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      GitOpsProjectName,
					Namespace: GitOpsProjectNamespace,
				},
				Spec: gitopsv1.GitOpsProjectSpec{
					URL:                 localRepository.Directory,
					PullIntervalSeconds: intervalInSeconds,
					Suspend:             &suspend,
				},
			}
		})

		When("The pull interval is less than 5 seconds", func() {
			It("Should not allow a pull interval less than 5 seconds", func() {
				ctx := context.Background()
				gitOpsProject.Spec.PullIntervalSeconds = 0

				err := k8sClient.Create(ctx, &gitOpsProject)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).Should(Equal("GitOpsProject.gitops.declcd.io \"test\" " +
					"is invalid: spec.pullIntervalSeconds: " +
					"Invalid value: 0: spec.pullIntervalSeconds in body should be greater than or equal to 5",
				))
			})
		})

		When("The pull interval is greater than or equal to 5 seconds", func() {
			AfterEach(func() {
				err := k8sClient.DeleteAllOf(ctx, &gitopsv1.GitOpsProject{}, client.InNamespace(GitOpsProjectNamespace))
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace("podinfo"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should do nothing when the project is suspended", func() {
				ctx := context.Background()
				suspend := true
				gitOpsProject.Spec.Suspend = &suspend

				Expect(k8sClient.Create(ctx, &gitOpsProject)).Should(Succeed())
				Expect(gitOpsProject.Spec.PullIntervalSeconds).To(Equal(intervalInSeconds))
				Expect(gitOpsProject.Spec.Suspend).To(Equal(&suspend))
				Expect(gitOpsProject.Spec.URL).To(Equal(localRepository.Directory))

				Consistently(func() error {
					var deployment appsv1.Deployment
					return k8sClient.Get(ctx, types.NamespacedName{Name: "podinfo", Namespace: "podinfo"}, &deployment)
				}, duration, assertionInterval).Should(MatchError(ContainSubstring("deployments.apps \"podinfo\" not found")))
			})

			It("Should reconcile the declared cluster state with the current cluster state", func() {
				ctx := context.Background()

				Expect(k8sClient.Create(ctx, &gitOpsProject)).Should(Succeed())
				Expect(gitOpsProject.Spec.PullIntervalSeconds).To(Equal(intervalInSeconds))
				Expect(gitOpsProject.Spec.Suspend).To(Equal(&suspend))
				Expect(gitOpsProject.Spec.URL).To(Equal(localRepository.Directory))

				By("Cloning a decl gitops repository, building manifests and applying them onto the cluster")
				Eventually(func() (string, error) {
					var deployment appsv1.Deployment
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: "podinfo", Namespace: "podinfo"}, &deployment); err != nil {
						return "", err
					}

					return deployment.GetName(), nil
				}, duration, assertionInterval).Should(Equal("podinfo"))

				Eventually(func() (*time.Time, error) {
					var updatedGitOpsProject gitopsv1.GitOpsProject
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: GitOpsProjectName, Namespace: GitOpsProjectNamespace}, &updatedGitOpsProject); err != nil {
						return nil, err
					}

					if updatedGitOpsProject.Status.LastPullTime == nil {
						return nil, nil
					}

					time := updatedGitOpsProject.Status.LastPullTime.Time
					return &time, nil
				}, duration, assertionInterval).ShouldNot(BeNil())
			})
		})

	})
})
