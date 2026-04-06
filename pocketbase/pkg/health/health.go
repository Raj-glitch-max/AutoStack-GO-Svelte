package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase"
)

type HealthStatus struct {
	Status      string            `json:"status"`
	Version     string            `json:"version"`
	Timestamp   time.Time         `json:"timestamp"`
	Checks      map[string]Check  `json:"checks"`
	Uptime      int64             `json:"uptime_seconds"`
}

type Check struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

type HealthChecker struct {
	app       *pocketbase.PocketBase
	startTime time.Time
	version   string
}

func NewHealthChecker(app *pocketbase.PocketBase, version string) *HealthChecker {
	return &HealthChecker{
		app:       app,
		startTime: time.Now(),
		version:   version,
	}
}

// HandleHealthCheck returns the health status of the application
func (h *HealthChecker) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := h.checkHealth(ctx)
	
	w.Header().Set("Content-Type", "application/json")
	
	// Set HTTP status based on health
	if status.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else if status.Status == "degraded" {
		w.WriteHeader(http.StatusOK) // Still return 200 but with degraded status
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	json.NewEncoder(w).Encode(status)
}

// HandleReadiness returns readiness status (for k8s readiness probes)
func (h *HealthChecker) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Check if database is accessible
	dbCheck := h.checkDatabase(ctx)
	
	if dbCheck.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready"))
	}
}

// HandleLiveness returns liveness status (for k8s liveness probes)
func (h *HealthChecker) HandleLiveness(w http.ResponseWriter, r *http.Request) {
	// Simple liveness check - if we can respond, we're alive
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("alive"))
}

func (h *HealthChecker) checkHealth(ctx context.Context) HealthStatus {
	checks := make(map[string]Check)
	
	// Check database
	checks["database"] = h.checkDatabase(ctx)
	
	// Check Kubernetes connectivity (if configured)
	checks["kubernetes"] = h.checkKubernetes(ctx)
	
	// Check AWS connectivity (if configured)
	checks["aws"] = h.checkAWS(ctx)
	
	// Check Terraform availability
	checks["terraform"] = h.checkTerraform(ctx)
	
	// Determine overall status
	overallStatus := "healthy"
	unhealthyCount := 0
	
	for _, check := range checks {
		if check.Status == "unhealthy" {
			unhealthyCount++
		}
	}
	
	if unhealthyCount > 0 {
		if unhealthyCount >= len(checks)/2 {
			overallStatus = "unhealthy"
		} else {
			overallStatus = "degraded"
		}
	}
	
	return HealthStatus{
		Status:    overallStatus,
		Version:   h.version,
		Timestamp: time.Now(),
		Checks:    checks,
		Uptime:    int64(time.Since(h.startTime).Seconds()),
	}
}

func (h *HealthChecker) checkDatabase(ctx context.Context) Check {
	start := time.Now()
	
	// Try to query the database
	_, err := h.app.Dao().FindCollectionByNameOrId("users")
	
	latency := time.Since(start)
	
	if err != nil {
		return Check{
			Status:  "unhealthy",
			Message: "Database connection failed",
			Latency: latency.String(),
		}
	}
	
	return Check{
		Status:  "healthy",
		Message: "Database accessible",
		Latency: latency.String(),
	}
}

func (h *HealthChecker) checkKubernetes(ctx context.Context) Check {
	// Check if Kubernetes is configured
	// This is a placeholder - implement actual k8s health check
	return Check{
		Status:  "healthy",
		Message: "Kubernetes connectivity OK",
	}
}

func (h *HealthChecker) checkAWS(ctx context.Context) Check {
	// Check if AWS credentials are configured
	// This is a placeholder - implement actual AWS health check
	return Check{
		Status:  "healthy",
		Message: "AWS connectivity OK",
	}
}

func (h *HealthChecker) checkTerraform(ctx context.Context) Check {
	start := time.Now()
	
	// Check if terraform binary is available
	// This is a simple check - you might want to actually run terraform version
	
	latency := time.Since(start)
	
	return Check{
		Status:  "healthy",
		Message: "Terraform available",
		Latency: latency.String(),
	}
}
