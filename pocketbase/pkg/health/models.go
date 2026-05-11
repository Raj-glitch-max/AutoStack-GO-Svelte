package health

// HealthStatus represents the status of a resource
type HealthStatusCode string

const (
	StatusHealthy   HealthStatusCode = "healthy"
	StatusUnhealthy HealthStatusCode = "unhealthy"
	StatusUnknown   HealthStatusCode = "unknown"
	StatusPending   HealthStatusCode = "pending"
	StatusDegraded  HealthStatusCode = "degraded"
)

// ResourceHealth contains health information for a specific resource (AWS or K8s)
type ResourceHealth struct {
	ResourceID   string           `json:"resourceId"`
	ResourceType string           `json:"resourceType"` // e.g., "ECS_SERVICE", "K8S_POD", "RDS_INSTANCE"
	Status       HealthStatusCode `json:"status"`
	Message      string           `json:"message"`
	Details      interface{}      `json:"details,omitempty"`
}

// DeploymentHealth contains aggregated health for a deployment
type DeploymentHealth struct {
	DeploymentID   string           `json:"deploymentId"`
	DeploymentType string           `json:"deploymentType"` // "aws" or "k8s"
	OverallStatus  HealthStatusCode `json:"overallStatus"`
	Resources      []ResourceHealth `json:"resources"`
}
