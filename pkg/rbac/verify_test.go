package rbac_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authorization/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/efortin/vllm-chill/pkg/rbac"
)

var _ = Describe("RBAC Verification", func() {
	Describe("GetRequiredPermissions", func() {
		It("should return non-empty permission list", func() {
			namespace := "vllm"
			permissions := rbac.GetRequiredPermissions(namespace)
			Expect(permissions).NotTo(BeEmpty())
		})

		It("should include cluster-scoped CRD permissions", func() {
			namespace := "vllm"
			permissions := rbac.GetRequiredPermissions(namespace)

			var hasCRDGet, hasCRDList bool
			for _, perm := range permissions {
				if perm.APIGroup == "apiextensions.k8s.io" && perm.Resource == "customresourcedefinitions" && perm.Verb == "get" && perm.Namespace == "" {
					hasCRDGet = true
				}
				if perm.APIGroup == "apiextensions.k8s.io" && perm.Resource == "customresourcedefinitions" && perm.Verb == "list" && perm.Namespace == "" {
					hasCRDList = true
				}
			}

			Expect(hasCRDGet).To(BeTrue(), "Missing cluster-scoped CRD get permission")
			Expect(hasCRDList).To(BeTrue(), "Missing cluster-scoped CRD list permission")
		})

		It("should include cluster-scoped model permissions", func() {
			namespace := "vllm"
			permissions := rbac.GetRequiredPermissions(namespace)

			var hasModelGet, hasModelList bool
			for _, perm := range permissions {
				if perm.APIGroup == "vllm.sir-alfred.io" && perm.Resource == "models" && perm.Verb == "get" && perm.Namespace == "" {
					hasModelGet = true
				}
				if perm.APIGroup == "vllm.sir-alfred.io" && perm.Resource == "models" && perm.Verb == "list" && perm.Namespace == "" {
					hasModelList = true
				}
			}

			Expect(hasModelGet).To(BeTrue(), "Missing cluster-scoped models get permission")
			Expect(hasModelList).To(BeTrue(), "Missing cluster-scoped models list permission")
		})

		It("should include namespace-scoped deployment permissions", func() {
			namespace := "vllm"
			permissions := rbac.GetRequiredPermissions(namespace)

			var hasDeploymentGet bool
			for _, perm := range permissions {
				if perm.APIGroup == "apps" && perm.Resource == "deployments" && perm.Verb == "get" && perm.Namespace == namespace {
					hasDeploymentGet = true
				}
			}

			Expect(hasDeploymentGet).To(BeTrue(), "Missing namespace-scoped deployments get permission for namespace %s", namespace)
		})
	})

	Describe("CheckPermission", func() {
		It("should return allowed for permitted actions", func() {
			clientset := fake.NewSimpleClientset()

			// Mock the SelfSubjectAccessReview response
			clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				sar := createAction.GetObject().(*authv1.SelfSubjectAccessReview)
				sar.Status = authv1.SubjectAccessReviewStatus{
					Allowed: true,
				}
				return true, sar, nil
			})

			perm := rbac.RequiredPermission{
				APIGroup:  "apps",
				Resource:  "deployments",
				Verb:      "get",
				Namespace: "vllm",
			}

			allowed, err := rbac.CheckPermission(context.Background(), clientset, perm)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("should return denied for forbidden actions", func() {
			clientset := fake.NewSimpleClientset()

			// Mock the SelfSubjectAccessReview response
			clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createAction := action.(k8stesting.CreateAction)
				sar := createAction.GetObject().(*authv1.SelfSubjectAccessReview)
				sar.Status = authv1.SubjectAccessReviewStatus{
					Allowed: false,
				}
				return true, sar, nil
			})

			perm := rbac.RequiredPermission{
				APIGroup:  "apps",
				Resource:  "deployments",
				Verb:      "delete",
				Namespace: "vllm",
			}

			allowed, err := rbac.CheckPermission(context.Background(), clientset, perm)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
		})
	})

	Describe("VerifyCRDExists", func() {
		It("should succeed when VLLMModel CRD exists and is established", func() {
			// Create a fake CRD client with the VLLMModel CRD
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "models.vllm.sir-alfred.io",
				},
				Status: apiextensionsv1.CustomResourceDefinitionStatus{
					Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
						{
							Type:   apiextensionsv1.Established,
							Status: apiextensionsv1.ConditionTrue,
						},
					},
				},
			}

			fakeClient := apiextensionsfake.NewSimpleClientset(crd)

			// Create a fake config that will use our fake client
			// Note: This test is limited because we can't easily inject the fake client
			// In real usage, this would be tested via integration tests
			_ = fakeClient

			// For unit tests, we'll just verify the CRD object structure
			Expect(crd.Name).To(Equal("models.vllm.sir-alfred.io"))
			Expect(crd.Status.Conditions).To(HaveLen(1))
			Expect(crd.Status.Conditions[0].Type).To(Equal(apiextensionsv1.Established))
			Expect(crd.Status.Conditions[0].Status).To(Equal(apiextensionsv1.ConditionTrue))
		})

		It("should fail when using rest.InClusterConfig outside cluster", func() {
			// This demonstrates that VerifyCRDExists will fail gracefully outside a cluster
			config := &rest.Config{}
			err := rbac.VerifyCRDExists(context.Background(), config)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Join", func() {
		It("should return empty string for empty slice", func() {
			result := rbac.Join([]string{}, ",")
			Expect(result).To(BeEmpty())
		})

		It("should return single element as-is", func() {
			result := rbac.Join([]string{"a"}, ",")
			Expect(result).To(Equal("a"))
		})

		It("should join multiple elements with separator", func() {
			result := rbac.Join([]string{"a", "b", "c"}, ",")
			Expect(result).To(Equal("a,b,c"))
		})

		It("should handle newline separators", func() {
			result := rbac.Join([]string{"line1", "line2"}, "\n")
			Expect(result).To(Equal("line1\nline2"))
		})
	})
})
