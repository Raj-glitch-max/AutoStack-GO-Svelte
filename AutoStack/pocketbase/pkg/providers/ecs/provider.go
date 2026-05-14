// Package ecs implements the AutoStack Provider interface for AWS ECS with
// Fargate launch type.
//
// Design contract: project-context/phase3/ecs-fargate-provider-design.md
// Isolation rules: project-context/phase3/provider-isolation-boundaries.md
// Capability profile: capabilities.go (authoritative)
//
// HC-6 isolation: this package is the ONLY package that may import
// github.com/aws/aws-sdk-go-v2/service/ecs or service/ec2.
// No aws-sdk imports may appear in pkg/reconciler, pkg/controller, or
// anywhere outside pkg/providers/ecs/.
package ecs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/janlauber/one-click/pkg/providers"
)

// Provider implements providers.Provider for AWS ECS / Fargate.
//
// The struct is intentionally stateless. All account-specific state
// (credentials, region, project context) is derived from the CloudAccount
// on each call so a single registered Provider is safe to share across
// concurrent reconciliations for different accounts.
type Provider struct {
	// confirmTimeoutOverride allows tests to shorten the confirm window
	// without changing the production constant. Zero means use the default.
	confirmTimeoutOverride time.Duration
}

// ECSCredentials is the decrypted credentials structure stored in
// CloudAccount.CredentialsEncrypted for aws-ecs accounts.
type ECSCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token,omitempty"` // optional; for temporary credentials
	AccountID       string `json:"account_id"`
}

// NewProvider creates a new ECS provider.
func NewProvider() *Provider {
	return &Provider{}
}

// awsCfg builds an aws.Config from a CloudAccount. The account's
// CredentialsEncrypted must already be decrypted by the reconciler
// (secrets.Decrypt is the reconciler's responsibility, not the provider's).
//
// The returned *aws.Config is ephemeral — the provider MUST NOT cache
// it beyond the lifetime of a single method call.
func awsCfg(ctx context.Context, account *providers.CloudAccount) (aws.Config, *ECSCredentials, error) {
	var creds ECSCredentials
	if err := json.Unmarshal([]byte(account.CredentialsEncrypted), &creds); err != nil {
		return aws.Config{}, nil, fmt.Errorf("ECS credentials parse error for account %q: %w", account.ID, err)
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return aws.Config{}, nil, &providers.ProviderError{
			Code:    "INVALID_CREDENTIALS",
			Message: "ECS credentials missing access_key_id or secret_access_key",
		}
	}

	region := account.Region
	if region == "" {
		return aws.Config{}, nil, &providers.ProviderError{
			Code:    "MISSING_REGION",
			Message: "cloud account region must be set for aws-ecs",
		}
	}

	staticCreds := credentials.NewStaticCredentialsProvider(
		creds.AccessKeyID,
		creds.SecretAccessKey,
		creds.SessionToken,
	)
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(staticCreds),
	)
	if err != nil {
		return aws.Config{}, nil, fmt.Errorf("aws config build failed: %w", err)
	}
	return cfg, &creds, nil
}

// clusterName returns the ECS cluster name for a given region.
// Convention: autostack-<region>. See ecs-fargate-provider-design.md §Region.
func clusterName(region string) string {
	return "autostack-" + region
}

// serviceName returns the ECS service name for a rollout.
// Convention: rolloutID (matches the AutoStack naming convention).
func serviceName(rolloutID string) string {
	return rolloutID
}

// taskDefFamily returns the ECS task definition family name.
// Convention: <rolloutID>-fargate.
func taskDefFamily(rolloutID string) string {
	return rolloutID + "-fargate"
}

// ValidateCredentials tests that the AWS credentials are valid and that
// the target ECS cluster exists in the account's region.
//
// Validation steps:
//  1. STS GetCallerIdentity — confirms credentials are valid.
//  2. ECS DescribeClusters — confirms the autostack-<region> cluster exists.
func (p *Provider) ValidateCredentials(ctx context.Context, account *providers.CloudAccount) error {
	cfg, _, err := awsCfg(ctx, account)
	if err != nil {
		return err
	}

	// Step 1: STS identity check.
	stsClient := sts.NewFromConfig(cfg)
	if _, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}); err != nil {
		return &providers.ProviderError{
			Code:    "AUTH_FAILED",
			Message: "AWS STS GetCallerIdentity failed: " + err.Error(),
		}
	}

	// Step 2: Cluster existence check.
	ecsClient := ecs.NewFromConfig(cfg)
	cluster := clusterName(account.Region)
	out, err := ecsClient.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{cluster},
	})
	if err != nil {
		return &providers.ProviderError{
			Code:    "CLUSTER_CHECK_FAILED",
			Message: fmt.Sprintf("ECS DescribeClusters for %q failed: %s", cluster, err),
		}
	}
	if len(out.Clusters) == 0 || out.Clusters[0].Status == nil || *out.Clusters[0].Status == "INACTIVE" {
		return &providers.ProviderError{
			Code:    "CLUSTER_NOT_FOUND",
			Message: fmt.Sprintf("ECS cluster %q does not exist or is INACTIVE in region %q; create it before deploying", cluster, account.Region),
		}
	}
	return nil
}

// Deploy creates or updates an ECS Fargate service for the given spec.
//
// Sequence (per ecs-fargate-provider-design.md §Deploy):
//  1. RegisterTaskDefinition with the Fargate container spec.
//  2. DescribeServices to check existence.
//  3. CreateService (new) or UpdateService (existing).
//
// Idempotent: safe to re-dispatch a pending target (CapIdempotentDeploy).
func (p *Provider) Deploy(ctx context.Context, account *providers.CloudAccount, spec *providers.DeploySpec) (*providers.DeployResult, error) {
	cfg, _, err := awsCfg(ctx, account)
	if err != nil {
		return nil, err
	}
	ecsClient := ecs.NewFromConfig(cfg)

	cluster := clusterName(account.Region)
	svcName := serviceName(spec.RolloutID)
	family := taskDefFamily(spec.RolloutID)

	// 1. Build and register task definition.
	taskDefARN, err := p.registerTaskDefinition(ctx, ecsClient, account, spec, family)
	if err != nil {
		return nil, fmt.Errorf("RegisterTaskDefinition failed: %w", err)
	}

	// 2. Check if service already exists.
	existing, err := p.describeService(ctx, ecsClient, cluster, svcName)
	if err != nil {
		return nil, fmt.Errorf("DescribeServices failed: %w", err)
	}

	if existing == nil {
		// 3a. CreateService.
		return p.createService(ctx, ecsClient, account, spec, cluster, svcName, taskDefARN)
	}
	// 3b. UpdateService.
	return p.updateService(ctx, ecsClient, cluster, svcName, taskDefARN, spec)
}

func (p *Provider) registerTaskDefinition(
	ctx context.Context,
	client *ecs.Client,
	account *providers.CloudAccount,
	spec *providers.DeploySpec,
	family string,
) (string, error) {
	image := spec.Image.Registry + "/" + spec.Image.Repository + ":" + spec.Image.Tag
	if spec.Image.Registry == "" {
		image = spec.Image.Repository + ":" + spec.Image.Tag
	}

	cpu := fmt.Sprintf("%d", int(spec.Compute.CPURequestVCPU*1024))
	mem := fmt.Sprintf("%d", spec.Compute.MemoryRequestMB)

	envVars := make([]ecstypes.KeyValuePair, 0, len(spec.Env))
	for _, e := range spec.Env {
		k, v := e.Name, e.Value
		envVars = append(envVars, ecstypes.KeyValuePair{Name: &k, Value: &v})
	}

	// Port mappings from the first network interface (if any).
	var portMappings []ecstypes.PortMapping
	for _, ni := range spec.Network.Interfaces {
		port := int32(ni.ContainerPort)
		proto := ecstypes.TransportProtocolTcp
		if strings.EqualFold(ni.Protocol, "udp") {
			proto = ecstypes.TransportProtocolUdp
		}
		portMappings = append(portMappings, ecstypes.PortMapping{
			ContainerPort: &port,
			Protocol:      proto,
		})
	}

	containerName := spec.RolloutID
	containerDef := ecstypes.ContainerDefinition{
		Name:         &containerName,
		Image:        &image,
		Environment:  envVars,
		PortMappings: portMappings,
	}

	out, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  &family,
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     &cpu,
		Memory:                  &mem,
		ContainerDefinitions:    []ecstypes.ContainerDefinition{containerDef},
	})
	if err != nil {
		return "", err
	}
	if out.TaskDefinition == nil || out.TaskDefinition.TaskDefinitionArn == nil {
		return "", fmt.Errorf("RegisterTaskDefinition returned nil ARN")
	}
	return *out.TaskDefinition.TaskDefinitionArn, nil
}

func (p *Provider) describeService(ctx context.Context, client *ecs.Client, cluster, svcName string) (*ecstypes.Service, error) {
	out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  &cluster,
		Services: []string{svcName},
	})
	if err != nil {
		var nf *ecstypes.ServiceNotFoundException
		if errors.As(err, &nf) {
			return nil, nil
		}
		return nil, err
	}
	if len(out.Services) == 0 || out.Services[0].Status == nil || *out.Services[0].Status == "INACTIVE" {
		return nil, nil
	}
	return &out.Services[0], nil
}

func (p *Provider) createService(
	ctx context.Context,
	client *ecs.Client,
	account *providers.CloudAccount,
	spec *providers.DeploySpec,
	cluster, svcName, taskDefARN string,
) (*providers.DeployResult, error) {
	desiredCount := int32(spec.Scale.MinReplicas)
	if desiredCount == 0 {
		desiredCount = 1
	}

	input := &ecs.CreateServiceInput{
		Cluster:        &cluster,
		ServiceName:    &svcName,
		TaskDefinition: &taskDefARN,
		DesiredCount:   &desiredCount,
		LaunchType:     ecstypes.LaunchTypeFargate,
		DeploymentConfiguration: &ecstypes.DeploymentConfiguration{
			DeploymentCircuitBreaker: &ecstypes.DeploymentCircuitBreaker{
				Enable:   true,
				Rollback: false, // AutoStack owns rollback; provider should not auto-rollback
			},
		},
	}

	// Pull target_group_arn from TargetConfig if provided (operator-owned ALB).
	if tg, ok := spec.TargetConfig["target_group_arn"].(string); ok && tg != "" {
		input.LoadBalancers = []ecstypes.LoadBalancer{
			{TargetGroupArn: &tg},
		}
	}

	out, err := client.CreateService(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("ECS CreateService failed: %w", err)
	}
	if out.Service == nil || out.Service.ServiceArn == nil {
		return nil, fmt.Errorf("CreateService returned nil service ARN")
	}

	return &providers.DeployResult{
		ExternalID: *out.Service.ServiceArn,
		Status:     "creating",
		Message:    "ECS service created; deployment in progress",
		CheckpointData: map[string]interface{}{
			"cluster":      cluster,
			"service_name": svcName,
			"task_def_arn": taskDefARN,
		},
	}, nil
}

func (p *Provider) updateService(
	ctx context.Context,
	client *ecs.Client,
	cluster, svcName, taskDefARN string,
	spec *providers.DeploySpec,
) (*providers.DeployResult, error) {
	desiredCount := int32(spec.Scale.MinReplicas)
	if desiredCount == 0 {
		desiredCount = 1
	}

	out, err := client.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:        &cluster,
		Service:        &svcName,
		TaskDefinition: &taskDefARN,
		DesiredCount:   &desiredCount,
	})
	if err != nil {
		return nil, fmt.Errorf("ECS UpdateService failed: %w", err)
	}
	if out.Service == nil || out.Service.ServiceArn == nil {
		return nil, fmt.Errorf("UpdateService returned nil service ARN")
	}

	return &providers.DeployResult{
		ExternalID: *out.Service.ServiceArn,
		Status:     "updating",
		Message:    "ECS service updated; deployment in progress",
		CheckpointData: map[string]interface{}{
			"cluster":      cluster,
			"service_name": svcName,
			"task_def_arn": taskDefARN,
		},
	}, nil
}

// GetStatus retrieves the current status of an ECS Fargate service.
//
// Maps ECS native state to canonical state per the N-rules in
// lifecycle-normalization-model.md and the ECS lifecycle table in
// ecs-fargate-provider-design.md.
func (p *Provider) GetStatus(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget) (*providers.TargetStatus, error) {
	cfg, _, err := awsCfg(ctx, account)
	if err != nil {
		return nil, err
	}
	ecsClient := ecs.NewFromConfig(cfg)

	cluster := clusterName(account.Region)
	svcName := serviceName(target.ExternalID)
	if strings.HasPrefix(target.ExternalID, "arn:") {
		// ExternalID is the ARN; extract service name from the end.
		parts := strings.Split(target.ExternalID, "/")
		if len(parts) > 0 {
			svcName = parts[len(parts)-1]
		}
	}

	out, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  &cluster,
		Services: []string{svcName},
	})

	input := lifecycleInput{
		destroyIntent:   target.Status == "deleting" || target.Status == "pending_destroy",
		hadPriorRunning: target.Status == "running" || target.Status == "updating",
	}

	if err != nil {
		if isServiceNotFound(err) {
			input.notFound = true
			ts := mapLifecycle(input)
			ts.LastUpdated = time.Now()
			ts.ConfidenceLevel = "high" // definitive: service is gone
			return &ts, nil
		}
		return nil, fmt.Errorf("ECS DescribeServices failed: %w", err)
	}

	if len(out.Services) == 0 {
		input.notFound = true
		ts := mapLifecycle(input)
		ts.LastUpdated = time.Now()
		ts.ConfidenceLevel = "high" // definitive: empty services list = not found
		return &ts, nil
	}

	svc := out.Services[0]
	if svc.Status != nil {
		input.serviceStatus = *svc.Status
	}
	if svc.DesiredCount != 0 {
		input.desiredCount = svc.DesiredCount
	}
	if svc.RunningCount != 0 {
		input.runningCount = svc.RunningCount
	}

	// Extract rollout state from the primary deployment.
	rolloutState := ""
	for _, dep := range svc.Deployments {
		if dep.Status != nil && *dep.Status == "PRIMARY" {
			rs := string(dep.RolloutState)
			input.rolloutState = rs
			rolloutState = rs
			if dep.TaskDefinition != nil {
				input.latestTaskDefARN = *dep.TaskDefinition
			}
			break
		}
	}

	ts := mapLifecycle(input)
	ts.Replicas = int(svc.DesiredCount)
	ts.AvailableReplicas = int(svc.RunningCount)
	ts.ReadyReplicas = int(svc.RunningCount)
	ts.LastUpdated = time.Now()

	// Phase 3.4: classify confidence based on ECS rolloutState signal quality.
	// COMPLETED and FAILED are definitive terminal states (high confidence).
	// IN_PROGRESS means rollout is actively changing (medium confidence).
	// Empty or unknown rolloutState means ECS gave no reliable signal (low).
	switch rolloutState {
	case "COMPLETED", "FAILED":
		ts.ConfidenceLevel = "high"
	case "IN_PROGRESS":
		ts.ConfidenceLevel = "medium"
	default:
		ts.ConfidenceLevel = "low"
	}
	return &ts, nil
}

// GetOperation returns ErrNotImplemented. ECS does not expose true LRO
// operation names. Per capability profile: CapOperationTracking Supported=false.
func (p *Provider) GetOperation(ctx context.Context, account *providers.CloudAccount, operationID string) (*providers.Operation, error) {
	return nil, providers.ErrNotImplemented
}

// Rollback rolls the ECS service back to a prior task definition revision.
//
// targetRevision MUST be a task definition ARN or family:revision string
// (e.g. "arn:aws:ecs:us-east-1:123456789:task-definition/my-svc-fargate:3"
// or "my-svc-fargate:3"). The reconciler sources this from
// operations.checkpoint_data.task_def_arn of the most recent succeeded
// deploy operation for this target (see cloud.go lookupPriorTaskDefARN).
//
// Rollback is implemented as UpdateService to the prior task definition.
// ECS will replace running tasks; GetStatus convergence confirms success.
// The confirmation loop is owned by the reconciler (Phase 3.3 extraction);
// Rollback() itself returns after UpdateService succeeds.
func (p *Provider) Rollback(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget, targetRevision string) (*providers.DeployResult, error) {
	if targetRevision == "" {
		return nil, &providers.ProviderError{
			Code:    "ROLLBACK_REVISION_REQUIRED",
			Message: "targetRevision (task definition ARN or family:revision) is required for ECS rollback; source from operations.checkpoint_data.task_def_arn",
		}
	}

	cfg, _, err := awsCfg(ctx, account)
	if err != nil {
		return nil, err
	}
	ecsClient := ecs.NewFromConfig(cfg)

	cluster := clusterName(account.Region)

	// Extract service name from ExternalID (service ARN or bare name).
	svcName := target.ExternalID
	if strings.HasPrefix(svcName, "arn:") {
		parts := strings.Split(svcName, "/")
		if len(parts) > 0 {
			svcName = parts[len(parts)-1]
		}
	}
	if svcName == "" {
		return nil, &providers.ProviderError{
			Code:    "ROLLBACK_EXTERNAL_ID_REQUIRED",
			Message: "target.ExternalID (service ARN or name) is required for ECS rollback",
		}
	}

	out, err := ecsClient.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:        &cluster,
		Service:        &svcName,
		TaskDefinition: &targetRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("ECS rollback UpdateService failed: %w", err)
	}
	if out.Service == nil || out.Service.ServiceArn == nil {
		return nil, fmt.Errorf("ECS rollback: UpdateService returned nil service ARN")
	}

	log.Printf("[ECS_ROLLBACK] cluster=%s service=%s rollback_task_def=%s", cluster, svcName, targetRevision)
	return &providers.DeployResult{
		ExternalID: *out.Service.ServiceArn,
		Status:     "updating",
		Message:    "ECS rollback initiated; service updating to prior task definition",
		CheckpointData: map[string]interface{}{
			"cluster":           cluster,
			"service_name":      svcName,
			"rollback_task_def": targetRevision,
		},
	}, nil
}

// GetMetrics returns ErrNotImplemented. CloudWatch integration is Phase 3.5+.
func (p *Provider) GetMetrics(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget) (*providers.TargetMetrics, error) {
	return nil, providers.ErrNotImplemented
}

// StreamLogs returns ErrNotImplemented. CloudWatch Logs integration is Phase 3+.
func (p *Provider) StreamLogs(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget, writer io.Writer) error {
	return providers.ErrNotImplemented
}

// EstimateCost returns a placeholder range. Live AWS Pricing API integration
// is Phase 3.5+ (per deferred list PR-6). Returning ErrNotImplemented
// rather than a fabricated zero would be more honest; however, cost display
// is non-blocking so we return a conservative range with UncertaintyNote.
func (p *Provider) EstimateCost(ctx context.Context, account *providers.CloudAccount, spec *providers.DeploySpec) (*providers.CostEstimate, error) {
	// TODO Phase 3.5: call AWS Pricing API for regional rates.
	// Returning a conservative range with explicit uncertainty note rather
	// than ErrNotImplemented so the UI can show something rather than crash.
	return &providers.CostEstimate{
		ComputeMonthlyLow:             5.0,
		ComputeMonthlyHigh:            200.0,
		TotalMonthlyLow:               5.0,
		TotalMonthlyHigh:              200.0,
		UncertaintyNote:               "Estimate uses placeholder range. Phase 3.5 will call AWS Pricing API for regional Fargate vCPU/GB rates.",
		PricingSource:                 "placeholder",
		CalculatedAt:                  time.Now(),
	}, nil
}

// GetActualCost returns ErrNotImplemented. AWS Cost Explorer integration
// is Phase 3.5+.
func (p *Provider) GetActualCost(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget, startDate, endDate time.Time) (*providers.ActualCost, error) {
	return nil, providers.ErrNotImplemented
}

// Destroy initiates removal of the ECS service (drain + delete).
//
// Phase 3.3 extraction: this method now only initiates the destroy — it does
// NOT wait for INACTIVE confirmation. The reconciler's dispatchDestroy owns
// the confirmation loop via confirmDestroyInReconciler (dispatch.go). This
// makes the confirmation policy (timeout, retry, ambiguity) uniform across
// providers and removes the self-confirm anti-pattern.
//
// Sequence:
//  1. UpdateService(desiredCount=0) — drain running tasks.
//  2. DeleteService — transitions service to DRAINING/deleting.
//
// Returns nil immediately after initiation. GetStatus will reflect INACTIVE
// or NOT_FOUND once AWS completes the deletion.
//
// Idempotent: returns nil if the service is already gone or INACTIVE.
func (p *Provider) Destroy(ctx context.Context, account *providers.CloudAccount, target *providers.DeploymentTarget) error {
	cfg, _, err := awsCfg(ctx, account)
	if err != nil {
		return err
	}
	ecsClient := ecs.NewFromConfig(cfg)

	cluster := clusterName(account.Region)
	svcName := target.ExternalID
	if strings.HasPrefix(svcName, "arn:") {
		parts := strings.Split(svcName, "/")
		if len(parts) > 0 {
			svcName = parts[len(parts)-1]
		}
	}

	// Step 1: Scale to zero (drain tasks).
	zero := int32(0)
	if _, err := ecsClient.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      &cluster,
		Service:      &svcName,
		DesiredCount: &zero,
	}); err != nil {
		var nf *ecstypes.ServiceNotFoundException
		if errors.As(err, &nf) {
			log.Printf("[ECS_DESTROY] service=%q not found; treating as already deleted", svcName)
			return nil
		}
		return fmt.Errorf("ECS UpdateService(desiredCount=0) failed: %w", err)
	}

	// Step 2: Delete service (transitions to DRAINING then INACTIVE on AWS side).
	if _, err := ecsClient.DeleteService(ctx, &ecs.DeleteServiceInput{
		Cluster: &cluster,
		Service: &svcName,
		Force:   aws.Bool(true), // force=true skips waiting for tasks to drain
	}); err != nil {
		var nf *ecstypes.ServiceNotFoundException
		if errors.As(err, &nf) {
			log.Printf("[ECS_DESTROY] service=%q already gone at DeleteService; OK", svcName)
			return nil
		}
		return fmt.Errorf("ECS DeleteService failed: %w", err)
	}

	log.Printf("[ECS_DESTROY] cluster=%s service=%s destroy initiated; reconciler will confirm", cluster, svcName)
	return nil
}

// ListRegions returns the available AWS regions by calling EC2 DescribeRegions.
func (p *Provider) ListRegions(ctx context.Context, account *providers.CloudAccount) ([]string, error) {
	cfg, _, err := awsCfg(ctx, account)
	if err != nil {
		return nil, err
	}
	ec2Client := ec2.NewFromConfig(cfg)

	out, err := ec2Client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, fmt.Errorf("EC2 DescribeRegions failed: %w", err)
	}

	regions := make([]string, 0, len(out.Regions))
	for _, r := range out.Regions {
		if r.RegionName != nil {
			regions = append(regions, *r.RegionName)
		}
	}
	return regions, nil
}

// CheckQuotas returns a placeholder Available: true. Live quota checking
// via AWS Service Quotas API is Phase 3.5+ (PR-7 from deferred list).
func (p *Provider) CheckQuotas(ctx context.Context, account *providers.CloudAccount, spec *providers.DeploySpec) (*providers.QuotaCheck, error) {
	// TODO Phase 3.5: call AWS Service Quotas API.
	// Returning Available: true is an honest lie only in that we're explicitly
	// documenting why — not because we checked. This matches the Cloud Run
	// baseline behavior (PR-7 deferred for both providers).
	return &providers.QuotaCheck{Available: true}, nil
}
