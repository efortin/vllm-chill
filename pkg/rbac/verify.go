package rbac

import (
	"context"
	"fmt"

	authv1 "k8s.io/api/authorization/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// RequiredPermission represents a permission that needs to be verified
type RequiredPermission struct {
	APIGroup  string
	Resource  string
	Verb      string
	Namespace string // empty for cluster-scoped
}

// GetRequiredPermissions returns the list of permissions required by vllm-chill
func GetRequiredPermissions(namespace string) []RequiredPermission {
	return []RequiredPermission{
		// Cluster-scoped permissions (CRDs)
		{APIGroup: "apiextensions.k8s.io", Resource: "customresourcedefinitions", Verb: "get", Namespace: ""},
		{APIGroup: "apiextensions.k8s.io", Resource: "customresourcedefinitions", Verb: "list", Namespace: ""},

		// Cluster-scoped permissions (VLLMModels CRD)
		{APIGroup: "vllm.sir-alfred.io", Resource: "models", Verb: "get", Namespace: ""},
		{APIGroup: "vllm.sir-alfred.io", Resource: "models", Verb: "list", Namespace: ""},

		// Namespace-scoped permissions
		{APIGroup: "apps", Resource: "deployments", Verb: "get", Namespace: namespace},
		{APIGroup: "apps", Resource: "deployments", Verb: "list", Namespace: namespace},
		{APIGroup: "apps", Resource: "deployments", Verb: "create", Namespace: namespace},
		{APIGroup: "apps", Resource: "deployments", Verb: "update", Namespace: namespace},
		{APIGroup: "apps", Resource: "deployments", Verb: "patch", Namespace: namespace},
		{APIGroup: "", Resource: "pods", Verb: "list", Namespace: namespace},
		{APIGroup: "", Resource: "pods", Verb: "delete", Namespace: namespace},
		{APIGroup: "", Resource: "pods", Verb: "deletecollection", Namespace: namespace},
		{APIGroup: "", Resource: "configmaps", Verb: "get", Namespace: namespace},
		{APIGroup: "", Resource: "configmaps", Verb: "create", Namespace: namespace},
		{APIGroup: "", Resource: "configmaps", Verb: "update", Namespace: namespace},
		{APIGroup: "", Resource: "configmaps", Verb: "patch", Namespace: namespace},
		{APIGroup: "", Resource: "services", Verb: "get", Namespace: namespace},
		{APIGroup: "", Resource: "services", Verb: "create", Namespace: namespace},
		{APIGroup: "", Resource: "services", Verb: "update", Namespace: namespace},
		{APIGroup: "", Resource: "services", Verb: "patch", Namespace: namespace},
	}
}

// VerifyPermissions checks if the current service account has all required permissions
func VerifyPermissions(ctx context.Context, namespace string) error {
	// Create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Verify VLLMModel CRD exists
	if err := VerifyCRDExists(ctx, config); err != nil {
		return err
	}

	permissions := GetRequiredPermissions(namespace)
	var missingPermissions []string

	for _, perm := range permissions {
		allowed, err := CheckPermission(ctx, clientset, perm)
		if err != nil {
			return fmt.Errorf("failed to check permission %s/%s:%s: %w", perm.APIGroup, perm.Resource, perm.Verb, err)
		}

		if !allowed {
			scope := "cluster-scoped"
			if perm.Namespace != "" {
				scope = fmt.Sprintf("namespace=%s", perm.Namespace)
			}
			missingPermissions = append(missingPermissions, fmt.Sprintf("  - %s %s.%s (%s)", perm.Verb, perm.Resource, perm.APIGroup, scope))
		}
	}

	if len(missingPermissions) > 0 {
		return fmt.Errorf("missing required RBAC permissions:\n%s\n\nPlease ensure the ServiceAccount has the required permissions as defined in manifests/ci/rbac.yaml",
			Join(missingPermissions, "\n"))
	}

	return nil
}

// VerifyCRDExists checks if the VLLMModel CRD is installed
func VerifyCRDExists(ctx context.Context, config *rest.Config) error {
	// Create API extensions clientset
	apiextensionsClient, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	// Check if VLLMModel CRD exists
	crdName := "models.vllm.sir-alfred.io"
	crd, err := apiextensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("VLLMModel CRD not found: %w\n\nPlease install the CRD using: kubectl apply -f manifests/crds/vllmmodel.yaml", err)
	}

	// Verify CRD is established
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue {
			return nil
		}
	}

	return fmt.Errorf("VLLMModel CRD exists but is not established\n\nPlease wait for the CRD to be fully initialized")
}

// CheckPermission verifies if a specific permission is granted
func CheckPermission(ctx context.Context, clientset kubernetes.Interface, perm RequiredPermission) (bool, error) {
	sar := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Verb:      perm.Verb,
				Group:     perm.APIGroup,
				Resource:  perm.Resource,
				Namespace: perm.Namespace,
			},
		},
	}

	result, err := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}

	return result.Status.Allowed, nil
}

// Join is a simple helper to join strings with a separator
func Join(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
