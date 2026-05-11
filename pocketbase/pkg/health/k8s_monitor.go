package health

import (
	"context"
	"fmt"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/k8s"
	"github.com/pocketbase/pocketbase/core"
)

// K8sHealthMonitor polls Kubernetes for resource health
type K8sHealthMonitor struct {
	app core.App
}

// NewK8sHealthMonitor creates a new Kubernetes health monitor
func NewK8sHealthMonitor(app core.App) *K8sHealthMonitor {
	return &K8sHealthMonitor{
		app: app,
	}
}

// GetDeploymentHealth aggregates health for all resources in a K8s deployment
func (km *K8sHealthMonitor) GetDeploymentHealth(ctx context.Context, deploymentID string) (*DeploymentHealth, error) {
	deployment, err := km.app.Dao().FindRecordById("deployments", deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to find deployment: %w", err)
	}

	projectID := deployment.GetString("project")
	
	res := &DeploymentHealth{
		DeploymentID:   deploymentID,
		DeploymentType: "k8s",
		OverallStatus:  StatusHealthy,
		Resources:      make([]ResourceHealth, 0),
	}

	// Fetch Rollout status from K8s
	status, err := k8s.GetRolloutStatus(projectID, deploymentID)
	if err != nil {
		res.OverallStatus = StatusUnknown
		res.Resources = append(res.Resources, ResourceHealth{
			ResourceID:   deploymentID,
			ResourceType: "K8S_ROLLOUT",
			Status:       StatusUnknown,
			Message:      fmt.Sprintf("Failed to fetch rollout status: %v", err),
		})
		return res, nil
	}

	// Map K8s rollout status to unified health
	rolloutHealth := ResourceHealth{
		ResourceID:   deploymentID,
		ResourceType: "K8S_ROLLOUT",
		Status:       mapK8sStatus(status.Deployment.Status),
		Message:      fmt.Sprintf("Replicas: %d, Pods: %d", status.Deployment.Replicas, len(status.Deployment.PodNames)),
		Details:      status,
	}
	res.Resources = append(res.Resources, rolloutHealth)
	res.OverallStatus = rolloutHealth.Status

	// Fetch Metrics if available
	metrics, err := k8s.GetRolloutMetrics(projectID, deploymentID)
	if err == nil && metrics != nil {
		res.Resources = append(res.Resources, ResourceHealth{
			ResourceID:   deploymentID + "-metrics",
			ResourceType: "K8S_METRICS",
			Status:       StatusHealthy,
			Message:      "Metrics collected successfully",
			Details:      metrics,
		})
	}

	return res, nil
}

func mapK8sStatus(k8sStatus string) HealthStatusCode {
	switch k8sStatus {
	case "Healthy", "Running", "available":
		return StatusHealthy
	case "Progressing", "Pending":
		return StatusPending
	case "Degraded", "Warning":
		return StatusDegraded
	case "Unhealthy", "Failed", "Error":
		return StatusUnhealthy
	default:
		return StatusUnknown
	}
}
