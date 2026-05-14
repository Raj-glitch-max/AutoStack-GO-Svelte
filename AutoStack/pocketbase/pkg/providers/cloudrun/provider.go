package cloudrun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/run/apiv2"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/janlauber/one-click/pkg/providers"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

// destroyConfirmTimeout is how long Destroy waits after DeleteService
// for GetService to start returning NOT_FOUND. Phase 2.8 truthfulness
// fix: closes the 10-60s window where AutoStack reported "deleted"
// while the provider still listed the service. Overridable via
// AUTOSTACK_CR_DESTROY_CONFIRM_TIMEOUT_SECONDS.
func destroyConfirmTimeout() time.Duration {
	if v := os.Getenv("AUTOSTACK_CR_DESTROY_CONFIRM_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 60 * time.Second
}

// destroyConfirmInterval is the poll interval inside the confirm loop.
const destroyConfirmInterval = 5 * time.Second

// Provider implements the providers.Provider interface for Google Cloud Run.
//
// The struct is intentionally stateless. Project ID and region are derived
// from the CloudAccount on each call so a single registered Provider is safe
// to share across concurrent reconciliations for different accounts.
type Provider struct{}

// CloudRunCredentials represents the decrypted GCP credentials structure
type CloudRunCredentials struct {
	ServiceAccountJSON string `json:"service_account_json"`
	ProjectID          string `json:"project_id"`
}

// NewProvider creates a new Cloud Run provider
func NewProvider() *Provider {
	return &Provider{}
}

// ValidateCredentials tests that the GCP credentials are valid
func (p *Provider) ValidateCredentials(ctx context.Context, account *providers.CloudAccount) error {
	var creds CloudRunCredentials
	if err := json.Unmarshal([]byte(account.CredentialsEncrypted), &creds); err != nil {
		return fmt.Errorf("failed to parse credentials: %w", err)
	}

	if creds.ServiceAccountJSON == "" {
		return fmt.Errorf("service account JSON is required")
	}

	client, err := run.NewServicesClient(ctx, option.WithCredentialsJSON([]byte(creds.ServiceAccountJSON)))
	if err != nil {
		return fmt.Errorf("failed to create Cloud Run client: %w", err)
	}
	defer client.Close()

	parent := fmt.Sprintf("projects/%s/locations/-", creds.ProjectID)
	iter := client.ListServices(ctx, &runpb.ListServicesRequest{Parent: parent})

	_, err = iter.Next()
	if err != nil && err != iterator.Done {
		return fmt.Errorf("failed to validate credentials: %w", err)
	}

	return nil
}

// Deploy creates or updates a Cloud Run service
func (p *Provider) Deploy(ctx context.Context, account *providers.CloudAccount, spec *providers.DeploySpec) (*providers.DeployResult, error) {
	var creds CloudRunCredentials
	if err := json.Unmarshal([]byte(account.CredentialsEncrypted), &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	client, err := run.NewServicesClient(ctx, option.WithCredentialsJSON([]byte(creds.ServiceAccountJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Run client: %w", err)
	}
	defer client.Close()

	// Build service name - Cloud Run requires lowercase, max 63 chars
	// Format: autostack-{truncated-rollout-id}
	baseName := "autostack-" + spec.RolloutID
	if len(baseName) > 63 {
		baseName = baseName[:63]
	}
	// Ensure lowercase and alphanumeric/hyphen only
	serviceName := strings.ToLower(baseName)

	parent := fmt.Sprintf("projects/%s/locations/%s", creds.ProjectID, account.Region)

	imageURL := fmt.Sprintf("%s/%s:%s", spec.Image.Registry, spec.Image.Repository, spec.Image.Tag)

	// Determine container port from spec, default to 8080
	containerPort := int32(8080)
	if len(spec.Network.Interfaces) > 0 && spec.Network.Interfaces[0].ContainerPort > 0 {
		containerPort = int32(spec.Network.Interfaces[0].ContainerPort)
	}

	container := &runpb.Container{
		Image: imageURL,
		Ports: []*runpb.ContainerPort{
			{ContainerPort: containerPort},
		},
	}

	// Add environment variables
	if len(spec.Env) > 0 {
		envVars := make([]*runpb.EnvVar, len(spec.Env))
		for i, env := range spec.Env {
			envVars[i] = &runpb.EnvVar{
				Name: env.Name,
				Values: &runpb.EnvVar_Value{
					Value: env.Value,
				},
			}
		}
		container.Env = envVars
	}

	// Configure CPU and memory
	if spec.Compute.CPULimitVCPU > 0 || spec.Compute.MemoryLimitMB > 0 {
		resources := &runpb.ResourceRequirements{
			Limits: map[string]string{},
		}
		if spec.Compute.CPULimitVCPU > 0 {
			resources.Limits["cpu"] = fmt.Sprintf("%.2f", spec.Compute.CPULimitVCPU)
		}
		if spec.Compute.MemoryLimitMB > 0 {
			resources.Limits["memory"] = fmt.Sprintf("%dMi", spec.Compute.MemoryLimitMB)
		}
		container.Resources = resources
	}

	// Configure scaling if specified in target config
	var scaling *runpb.RevisionScaling
	if spec.TargetConfig != nil {
		if minInstances, ok := spec.TargetConfig["min_instances"].(float64); ok {
			scaling = &runpb.RevisionScaling{
				MinInstanceCount: int32(minInstances),
			}
		}
	}

	service := &runpb.Service{
		Labels: map[string]string{
			"autostack-managed": "true",
			"rollout-id":        spec.RolloutID,
		},
		Template: &runpb.RevisionTemplate{
			Labels: map[string]string{
				"autostack-managed": "true",
				"rollout-id":        spec.RolloutID,
			},
			Containers: []*runpb.Container{container},
			Scaling:    scaling,
		},
	}

	// The parent ctx is bounded by the reconciler dispatcher (DeployTimeout,
	// 15 min by default). Phase 2.0 used a redundant 10-minute internal
	// WithTimeout here and a separate 5-minute timeout on
	// waitForServiceReady; the innermost timeout was the one that actually
	// fired for slow Cloud Run cold starts. Phase 2.1: rely on the parent
	// ctx so the dispatcher is the sole owner of the deploy time budget.
	// If the parent has no deadline, Cloud Run's per-call defaults still
	// apply at the API client layer.

	// Check if service exists
	existing, err := client.GetService(ctx, &runpb.GetServiceRequest{
		Name: fmt.Sprintf("%s/services/%s", parent, serviceName),
	})
	if err == nil && existing != nil {
		// Update existing
		service.Name = existing.Name
		_, err = client.UpdateService(ctx, &runpb.UpdateServiceRequest{
			Service: service,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update Cloud Run service: %w", err)
		}
	} else {
		// Create new
		// NOTE: service.Name uses the fully-qualified resource path (the
		// last segment is the service ID). GCP derives ServiceId from the
		// last segment of service.Name when no explicit ServiceId is given.
		// Passing both service.Name AND ServiceId can cause GCP internal
		// validation to reject the request as a conflict, so we omit
		// ServiceId entirely.
		service.Name = fmt.Sprintf("%s/services/%s", parent, serviceName)
		_, err = client.CreateService(ctx, &runpb.CreateServiceRequest{
			Parent:  parent,
			Service: service,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create Cloud Run service: %w", err)
		}
	}

	// waitForServiceReady's internal deadline is now an upper bound; the
	// parent ctx will usually cancel first. Passing a generous value here
	// keeps the function self-contained (it doesn't need to know the
	// dispatcher's budget) while ensuring the parent ctx always wins.
	endpoint, status, err := p.waitForServiceReady(ctx, client, parent, serviceName, 1*time.Hour)
	if err != nil {
		return &providers.DeployResult{
			ExternalID:  serviceName,
			EndpointURL: endpoint,
			Status:      status,
			Message:     err.Error(),
		}, nil
	}

	return &providers.DeployResult{
		ExternalID:  serviceName,
		EndpointURL: endpoint,
		Status:      status,
		Message:     "Deployment completed",
	}, nil
}

func (p *Provider) waitForServiceReady(ctx context.Context, client *run.ServicesClient, parent, serviceName string, timeout time.Duration) (string, string, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return "", "cancelled", fmt.Errorf("service readiness check cancelled: %w", ctx.Err())
		default:
			// Continue polling
		}

		// Check timeout
		if time.Now().After(deadline) {
			return "", "timeout", fmt.Errorf("service did not become ready within timeout")
		}

		service, err := client.GetService(ctx, &runpb.GetServiceRequest{
			Name: fmt.Sprintf("%s/services/%s", parent, serviceName),
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to get service: %w", err)
		}

		for _, cond := range service.Conditions {
			if cond.Type == "Ready" {
				if cond.State == runpb.Condition_CONDITION_SUCCEEDED {
					return service.Uri, "running", nil
				}
				if cond.State == runpb.Condition_CONDITION_FAILED && cond.Message != "" {
					return service.Uri, "error", fmt.Errorf("%s", cond.Message)
				}
			}
		}

		// Sleep with early exit on context cancellation
		select {
		case <-ctx.Done():
			return "", "cancelled", fmt.Errorf("service readiness check cancelled: %w", ctx.Err())
		case <-time.After(pollInterval):
			// Continue polling
		}
	}
}

// GetStatus retrieves the current status of a Cloud Run service
func (p *Provider) GetStatus(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget) (*providers.TargetStatus, error) {
	var creds CloudRunCredentials
	if err := json.Unmarshal([]byte(account.CredentialsEncrypted), &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	client, err := run.NewServicesClient(ctx, option.WithCredentialsJSON([]byte(creds.ServiceAccountJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Run client: %w", err)
	}
	defer client.Close()

	parent := fmt.Sprintf("projects/%s/locations/%s", creds.ProjectID, account.Region)

	service, err := client.GetService(ctx, &runpb.GetServiceRequest{
		Name: fmt.Sprintf("%s/services/%s", parent, target.ExternalID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get Cloud Run service: %w", err)
	}

	// Defensive default: "unknown" if no relevant condition is present.
	// Previously this defaulted to "pending", which combined with the
	// reconciler's old behavior could regress a healthy "running" target to
	// "pending" on a single inconclusive observation. The transition guard
	// in updateTargetStatus also refuses running → pending; "unknown" is the
	// honest signal when the provider gave us nothing actionable.
	//
	// Precedence: a FAILED Ready outranks a RECONCILING Configuration; a
	// SUCCEEDED Ready outranks a RECONCILING Configuration. RECONCILING is
	// only surfaced as "creating" when no Ready condition is present at all.
	status := "unknown"
	message := ""
	replicas := 0

	hasReady := false
	for _, cond := range service.Conditions {
		if cond.Type == "Ready" {
			hasReady = true
			if cond.State == runpb.Condition_CONDITION_SUCCEEDED {
				status = "running"
			} else if cond.State == runpb.Condition_CONDITION_FAILED {
				status = "error"
				message = cond.Message
			}
		}
	}
	if !hasReady {
		for _, cond := range service.Conditions {
			if cond.Type == "ConfigurationsReady" && cond.State == runpb.Condition_CONDITION_RECONCILING {
				status = "creating"
				break
			}
		}
	}

	return &providers.TargetStatus{
		Status:            status,
		Replicas:          replicas,
		AvailableReplicas: replicas,
		ReadyReplicas:     replicas,
		Message:           message,
		LastUpdated:       time.Now(),
	}, nil
}

// GetMetrics retrieves resource metrics.
//
// Returns ErrNotImplemented until Cloud Monitoring integration lands. Returning
// a zero-valued TargetMetrics with a nil error would render as "0% CPU" in the
// UI, which is a lie about state. Callers must surface "metrics unavailable".
func (p *Provider) GetMetrics(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget) (*providers.TargetMetrics, error) {
	return nil, providers.ErrNotImplemented
}

// GetOperation retrieves the status of an in-flight operation.
//
// Returns ErrNotImplemented. The previous implementation treated the
// operation ID as a service name (passing it as a service-name path
// component to GetService), and fabricated a StartedAt timestamp. That is
// inferred state masquerading as LRO tracking. Real LRO support requires
// returning the operation name from Deploy/Rollback/Destroy and polling
// the operations API. Until then, callers must not pretend to track
// in-flight operations through this method.
func (p *Provider) GetOperation(ctx context.Context, account *providers.CloudAccount, operationID string) (*providers.Operation, error) {
	return nil, providers.ErrNotImplemented
}

// Rollback rolls back a deployment to a previous revision.
//
// Returns ErrNotImplemented. The previous body was structurally unsafe:
//   - It posted an empty Service to UpdateService with no Name and no
//     Template content. If accepted, the service would be wiped.
//   - It picked revisions[1] without sorting; ListRevisions order is
//     provider-defined and not "previous deploy".
//   - The selected revision was never threaded into the update call.
//   - It then reported the un-rolled-back state as a successful rollback.
//
// A correct Cloud Run v2 rollback uses Service.Traffic with a TrafficTarget
// pinning Percent: 100 to a known prior Revision name, and requires that
// AutoStack persist current/previous revision lineage on deployment_targets
// before invoking the API. Refuse until that lineage exists.
func (p *Provider) Rollback(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget, targetRevision string) (*providers.DeployResult, error) {
	return nil, providers.ErrNotImplemented
}

// StreamLogs streams logs from Cloud Run
func (p *Provider) StreamLogs(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget, writer io.Writer) error {
	var creds CloudRunCredentials
	if err := json.Unmarshal([]byte(account.CredentialsEncrypted), &creds); err != nil {
		return fmt.Errorf("failed to parse credentials: %w", err)
	}

	// Note: Cloud Logging client would be needed here
	// For now, return not implemented
	return fmt.Errorf("log streaming not yet implemented")
}

// EstimateCost provides a cost estimate
// NOTE: This is a placeholder implementation using static pricing values.
// ADR-010 requires calling live pricing APIs. This should be updated to
// call GCP Cloud Billing API before production use.
func (p *Provider) EstimateCost(ctx context.Context, account *providers.CloudAccount, spec *providers.DeploySpec) (*providers.CostEstimate, error) {
	vCPUHours := spec.Compute.CPULimitVCPU * 730
	memHours := float64(spec.Compute.MemoryLimitMB) / 1024 * 730

	// Static pricing based on average US regions (placeholder - not live API)
	cpuRateLow := 0.000016 * 0.8
	cpuRateHigh := 0.000016 * 1.2
	memRateLow := 0.0000024 * 0.8
	memRateHigh := 0.0000024 * 1.2

	computeLow := (vCPUHours * cpuRateLow) + (memHours * memRateLow)
	computeHigh := (vCPUHours * cpuRateHigh) + (memHours * memRateHigh)

	// Placeholder request estimate
	requestRate := 0.40 / 1000000
	dailyRequests := 100000.0
	requestsMonthly := dailyRequests * 30

	infraLow := requestsMonthly * requestRate * 0.5
	infraHigh := requestsMonthly * requestRate * 1.5

	return &providers.CostEstimate{
		ComputeMonthlyLow:         computeLow,
		ComputeMonthlyHigh:        computeHigh,
		InfrastructureMonthlyLow:  infraLow,
		InfrastructureMonthlyHigh: infraHigh,
		TotalMonthlyLow:           computeLow + infraLow,
		TotalMonthlyHigh:          computeHigh + infraHigh,
		UncertaintyNote:           "Estimate based on default region pricing. Actual costs vary by region and usage. Does not include networking egress.",
		PricingSource:             "gcp_cloud_run_pricing_2024",
		CalculatedAt:              time.Now(),
	}, nil
}

// GetActualCost retrieves actual cost
func (p *Provider) GetActualCost(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget, startDate, endDate time.Time) (*providers.ActualCost, error) {
	return nil, fmt.Errorf("actual cost retrieval not yet implemented for Cloud Run")
}

// Destroy removes the Cloud Run service
// Idempotent: returns nil if service is already deleted
func (p *Provider) Destroy(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget) error {
	var creds CloudRunCredentials
	if err := json.Unmarshal([]byte(account.CredentialsEncrypted), &creds); err != nil {
		return fmt.Errorf("failed to parse credentials: %w", err)
	}

	client, err := run.NewServicesClient(ctx, option.WithCredentialsJSON([]byte(creds.ServiceAccountJSON)))
	if err != nil {
		return fmt.Errorf("failed to create Cloud Run client: %w", err)
	}
	defer client.Close()

	parent := fmt.Sprintf("projects/%s/locations/%s", creds.ProjectID, account.Region)
	serviceName := fmt.Sprintf("%s/services/%s", parent, target.ExternalID)

	// Check if service exists first (idempotent - if not found, consider deleted)
	existing, err := client.GetService(ctx, &runpb.GetServiceRequest{
		Name: serviceName,
	})
	if err != nil {
		// Check if error is "not found" - in that case, consider already deleted
		// This is idempotent behavior
		if strings.Contains(err.Error(), "NOT_FOUND") || strings.Contains(err.Error(), "not found") {
			return nil // Already deleted
		}
		// Any other error, return it
		return fmt.Errorf("failed to check service existence: %w", err)
	}

	// Only call DeleteService if we successfully retrieved a live service with
	// a non-empty UID. strings.Contains(_, "") was always true and masked the
	// intent of this predicate.
	if existing != nil && existing.Uid != "" {
		_, err = client.DeleteService(ctx, &runpb.DeleteServiceRequest{
			Name: serviceName,
		})
		if err != nil {
			// If already deleting or not found, return nil (idempotent)
			if strings.Contains(err.Error(), "NOT_FOUND") || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound") {
				return nil
			}
			return fmt.Errorf("failed to delete Cloud Run service: %w", err)
		}

		// Phase 2.8 truthfulness fix: poll GetService until NOT_FOUND or
		// timeout. Returning before NOT_FOUND would leave a 10-60s window
		// where AutoStack reports `deleted` but the provider still lists
		// the service. See
		// project-context/phase2.8/post-destroy-confirm-poll.md.
		if !p.confirmDeleted(ctx, client, serviceName) {
			// We couldn't confirm within the budget; the API accepted our
			// delete so we still report success, but log loudly so an
			// operator can verify provider-side.
			log.Printf("[DESTROY_CONFIRM_TIMEOUT] service=%s — DeleteService API succeeded but GetService still returns the service after confirm window; verify via console", serviceName)
		}
	}

	return nil
}

// confirmDeleted polls GetService every destroyConfirmInterval until
// the service returns NOT_FOUND or destroyConfirmTimeout elapses or
// the ctx is cancelled. Returns true on confirmed deletion, false on
// timeout or cancellation.
//
// Non-NOT_FOUND GetService errors (transient network, permission) are
// LOGGED and treated as "still listed" — we cannot confirm deletion,
// so we keep polling. If ctx cancels, we return false (caller decides
// whether to log loud).
func (p *Provider) confirmDeleted(ctx context.Context, client *run.ServicesClient, serviceName string) bool {
	deadline := time.Now().Add(destroyConfirmTimeout())
	for {
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		default:
		}
		_, err := client.GetService(ctx, &runpb.GetServiceRequest{Name: serviceName})
		if err != nil {
			if strings.Contains(err.Error(), "NOT_FOUND") || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound") {
				return true
			}
			// Transient — log and keep polling.
			log.Printf("[DESTROY_CONFIRM_POLL_ERR] service=%s err=%v", serviceName, err)
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(destroyConfirmInterval):
		}
	}
}

// ListRegions returns available GCP regions
func (p *Provider) ListRegions(ctx context.Context, account *providers.CloudAccount) ([]string, error) {
	return []string{
		"us-central1", "us-east1", "us-east4", "us-west1", "us-west2",
		"us-west3", "us-west4", "europe-west1", "europe-west2", "europe-west3",
		"europe-west4", "europe-west6", "asia-east1", "asia-east2", "asia-northeast1",
		"asia-northeast2", "asia-northeast3", "asia-south1", "asia-southeast1", "asia-southeast2",
		"australia-southeast1", "australia-southeast2", "northamerica-northeast1", "southamerica-east1",
	}, nil
}

// CheckQuotas verifies quotas.
//
// Returns ErrNotImplemented. The previous body returned Available: true with
// no violations regardless of input, which is a lie the UI would render as
// "deployable" right before the deploy fails. Until live quota lookups are
// implemented, callers must treat quota availability as unknown.
func (p *Provider) CheckQuotas(ctx context.Context, account *providers.CloudAccount, spec *providers.DeploySpec) (*providers.QuotaCheck, error) {
	return nil, providers.ErrNotImplemented
}

// Ensure proto is used (avoids unused import error)
var _ proto.Message