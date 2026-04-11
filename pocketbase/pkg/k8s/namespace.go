package k8s

import (
	"fmt"
	"strings"

	"github.com/Raj-glitch-max/autostack/pkg/util"
	"github.com/pocketbase/pocketbase/models"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespaceParams struct {
	Name       string
	UserRecord *models.Record
	ProjectID  string // WEEK 2 TASK 2.5: Added for project-specific namespaces
}

// CreateNamespace creates a namespace for a project with resource quotas
// WEEK 2 TASK 2.5: Fix K8s namespace isolation - each project gets its own namespace
func CreateNamespace(params NamespaceParams) error {
	// WEEK 2 TASK 2.5: Generate namespace name from project ID
	// Format: proj-{shortProjectID} (DNS-safe, lowercase)
	namespaceName := params.Name
	if namespaceName == "" && params.ProjectID != "" {
		namespaceName = fmt.Sprintf("proj-%s", strings.ToLower(params.ProjectID[:8]))
	}

	// Validate namespace name is DNS-safe
	if !isDNSSafe(namespaceName) {
		return fmt.Errorf("invalid namespace name: must be DNS-safe (lowercase alphanumeric and hyphens)")
	}

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				"one-click.dev/projectId":   params.ProjectID,
				"one-click.dev/userId":      params.UserRecord.GetString("id"),
				"one-click.dev/username":    params.UserRecord.GetString("username"),
				"one-click.dev/displayName": util.StringParser(params.UserRecord.GetString("name")),
				"one-click.dev/managed":     "true",
			},
		},
	}
	
	_, err := Clientset.CoreV1().Namespaces().Create(Ctx, ns, metav1.CreateOptions{})

	// if err already exists, update
	if err != nil && strings.Contains(err.Error(), "already exists") {
		_, err = Clientset.CoreV1().Namespaces().Update(Ctx, ns, metav1.UpdateOptions{})
	}

	// WEEK 2 TASK 2.5: Create ResourceQuota for the namespace
	if err == nil {
		if err := createResourceQuota(namespaceName); err != nil {
			// Log error but don't fail namespace creation
			fmt.Printf("Warning: Failed to create resource quota for namespace %s: %v\n", namespaceName, err)
		}
	}

	return err
}

// createResourceQuota creates a ResourceQuota for a namespace
// WEEK 2 TASK 2.5: Add ResourceQuota per namespace (default limits)
func createResourceQuota(namespace string) error {
	quota := &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-quota",
			Namespace: namespace,
			Labels: map[string]string{
				"one-click.dev/managed": "true",
			},
		},
		Spec: v1.ResourceQuotaSpec{
			Hard: v1.ResourceList{
				// CPU limits
				v1.ResourceRequestsCPU: resource.MustParse("2"),
				v1.ResourceLimitsCPU:   resource.MustParse("4"),
				// Memory limits
				v1.ResourceRequestsMemory: resource.MustParse("2Gi"),
				v1.ResourceLimitsMemory:   resource.MustParse("4Gi"),
				// Pod limits
				v1.ResourcePods: resource.MustParse("10"),
			},
		},
	}

	_, err := Clientset.CoreV1().ResourceQuotas(namespace).Create(Ctx, quota, metav1.CreateOptions{})
	
	// If quota already exists, update it
	if err != nil && strings.Contains(err.Error(), "already exists") {
		_, err = Clientset.CoreV1().ResourceQuotas(namespace).Update(Ctx, quota, metav1.UpdateOptions{})
	}

	return err
}

// DeleteNamespace deletes a namespace
func DeleteNamespace(namespace string) error {
	return Clientset.CoreV1().Namespaces().Delete(Ctx, namespace, metav1.DeleteOptions{})
}

// isDNSSafe checks if a string is DNS-safe (lowercase alphanumeric and hyphens)
// WEEK 2 TASK 2.5: Validate namespace names are DNS-safe
func isDNSSafe(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	
	// Must start with lowercase letter
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}
	
	// Must end with lowercase letter or digit
	last := name[len(name)-1]
	if !((last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')) {
		return false
	}
	
	// Check all characters are lowercase alphanumeric or hyphen
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	
	return true
}

// GetProjectNamespace returns the namespace name for a project
// WEEK 2 TASK 2.5: Helper function to get project namespace
func GetProjectNamespace(projectID string) string {
	if len(projectID) >= 8 {
		return fmt.Sprintf("proj-%s", strings.ToLower(projectID[:8]))
	}
	return fmt.Sprintf("proj-%s", strings.ToLower(projectID))
}
