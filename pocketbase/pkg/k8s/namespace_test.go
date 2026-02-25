package k8s

import (
	"testing"
)

// WEEK 2 TASK 2.5: Test DNS-safe namespace validation
func TestIsDNSSafe(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid lowercase", "proj-abc123", true},
		{"valid short", "proj-a", true},
		{"invalid uppercase", "Proj-ABC", false},
		{"invalid starts with number", "1proj", false},
		{"invalid starts with hyphen", "-proj", false},
		{"invalid ends with hyphen", "proj-", false},
		{"invalid special char", "proj_abc", false},
		{"invalid empty", "", false},
		{"invalid too long", "proj-" + string(make([]byte, 60)), false},
		{"valid max length", "proj-" + string(make([]byte, 57)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDNSSafe(tt.input)
			if result != tt.expected {
				t.Errorf("isDNSSafe(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// WEEK 2 TASK 2.5: Test project namespace generation
func TestGetProjectNamespace(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		expected  string
	}{
		{"standard project ID", "ABC12345XYZ", "proj-abc12345"},
		{"short project ID", "ABC", "proj-abc"},
		{"lowercase project ID", "abc12345", "proj-abc12345"},
		{"mixed case project ID", "AbC12345", "proj-abc12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetProjectNamespace(tt.projectID)
			if result != tt.expected {
				t.Errorf("GetProjectNamespace(%q) = %q, want %q", tt.projectID, result, tt.expected)
			}
		})
	}
}

// WEEK 2 TASK 2.5: Test namespace isolation concept
func TestNamespaceIsolation(t *testing.T) {
	t.Log("WEEK 2 TASK 2.5: K8s Namespace Isolation")
	t.Log("Each project gets its own namespace: proj-{shortProjectID}")
	t.Log("Namespace names are DNS-safe (lowercase alphanumeric and hyphens)")
	t.Log("ResourceQuota is created per namespace with default limits:")
	t.Log("  - CPU requests: 2 cores, limits: 4 cores")
	t.Log("  - Memory requests: 2Gi, limits: 4Gi")
	t.Log("  - Max pods: 10")
	t.Log("Never use 'default' namespace for user deployments")
}
