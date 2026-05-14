package providers

import (
	"context"
	"io"
	"time"

	pb_models "github.com/pocketbase/pocketbase/models"
)

// CloudAccount represents a cloud provider account stored in PocketBase.
//
// CredentialsEncrypted carries either the v1 ciphertext form ("enc:v1:...")
// or, during a Provider call, the in-memory plaintext returned by
// secrets.Decrypt. The field name reflects what is at rest, NOT what is
// at hand. Treat the contents as sensitive for the entire lifetime of
// the struct.
type CloudAccount struct {
	ID                   string
	Name                 string
	User                 *pb_models.Record
	Provider             string // aws, gcp, azure
	Region               string
	CredentialsEncrypted string // JSON blob, decrypted at runtime
	Status               string // active, error, validating, revoked
	ValidatedAt          time.Time
	LastValidated        time.Time
	ValidationError      string
}

// String returns an operator-safe representation of the account. The
// credential blob is redacted so accidental %v / %+v / structured log
// invocations cannot leak plaintext credentials. Always prefer this
// over the default fmt printers.
func (a CloudAccount) String() string {
	return "CloudAccount{ID=" + a.ID +
		" Name=" + a.Name +
		" Provider=" + a.Provider +
		" Region=" + a.Region +
		" Status=" + a.Status +
		" CredentialsEncrypted=<REDACTED>}"
}

// GoString delegates to String so %#v also redacts.
func (a CloudAccount) GoString() string { return a.String() }

// DeploymentTarget represents a cloud deployment target in PocketBase
type DeploymentTarget struct {
	ID            string
	Rollout       *pb_models.Record
	CloudAccount  *pb_models.Record
	Provider      string // aws-ecs, gcp-cloudrun, azure-aca
	Region        string
	ExternalID    string // Cloud-specific resource identifier
	Status        string // pending, creating, running, updating, stopped, error, deleting, deleted
	EndpointURL   string
	LastSynced    time.Time
	DriftDetected bool
	DriftSummary  string
}

// DeploySpec contains the specification for deploying to a cloud provider
type DeploySpec struct {
	RolloutID       string
	Image           ImageSpec
	Compute         ComputeSpec
	Scale           ScaleSpec
	Network         NetworkSpec
	Env             []EnvVar
	Secrets         []SecretRef
	Health          *HealthSpec
	TargetConfig    map[string]interface{} // Provider-specific overrides
}

// ImageSpec defines the container image to deploy
type ImageSpec struct {
	Registry           string
	Repository         string
	Tag                string
	PullPolicy         string
	RegistryCredential string // PocketBase ID for registry credentials
}

// ComputeSpec defines CPU and memory requirements
type ComputeSpec struct {
	CPURequestVCPU    float64
	CPULimitVCPU      float64
	MemoryRequestMB   int
	MemoryLimitMB     int
}

// ScaleSpec defines scaling configuration
type ScaleSpec struct {
	MinReplicas        int
	MaxReplicas        int
	ScaleMetric        string // cpu, memory, requests
	ScaleTargetPercent int
}

// NetworkSpec defines network configuration
type NetworkSpec struct {
	Interfaces []NetworkInterface
}

// NetworkInterface defines a single network endpoint
type NetworkInterface struct {
	ContainerPort  int
	Protocol       string
	ServiceType    string // LoadBalancer, ClusterIP, etc.
	IngressHost    string
	TLSEnabled     bool
}

// EnvVar defines an environment variable
type EnvVar struct {
	Name  string
	Value string
}

// SecretRef defines a reference to an external secret
type SecretRef struct {
	Name        string
	Source      string // kubernetes-secret, aws-secrets-manager, gcp-secret-manager, azure-key-vault
	SecretName  string
	SecretKey   string
}

// HealthSpec defines health check configuration
type HealthSpec struct {
	LivenessPath         string
	LivenessPort         int
	ReadinessPath        string
	ReadinessPort        int
	InitialDelaySeconds int
	PeriodSeconds       int
	FailureThreshold     int
}

// OperationStatus represents the state of an in-flight operation
type OperationStatus string

const (
	OperationStatusPending    OperationStatus = "pending"
	OperationStatusInProgress OperationStatus = "in_progress"
	OperationStatusSucceeded OperationStatus = "succeeded"
	OperationStatusFailed    OperationStatus = "failed"
	OperationStatusCancelled OperationStatus = "cancelled"
)

// Operation represents an in-flight deployment operation
type Operation struct {
	ID        string
	Status    OperationStatus
	Message   string
	StartedAt time.Time
	UpdatedAt time.Time
}

// DeployResult contains the result of a deployment operation
type DeployResult struct {
	ExternalID    string
	EndpointURL   string
	Status        string
	Message       string
	OperationID   string // For tracking in-flight operations
}

// TargetStatus contains the current status of a deployment.
//
// Phase 3 (Form B additive evolution): ServingRevision was added to
// distinguish "a revision is ready" from "this revision is receiving
// traffic." See phase3/provider-contract-evolution.md Change 3. Phase 2
// callers that ignore the field continue to function unchanged.
type TargetStatus struct {
	Status        string
	Replicas      int
	AvailableReplicas int
	ReadyReplicas int
	Message       string
	LastUpdated   time.Time

	// ServingRevision is the revision currently receiving traffic, when
	// the provider can report it (Capability C-HealthReporting must be
	// Supported: true). Empty string means the provider cannot report
	// it — the caller must NOT infer "no revision serving" from empty;
	// it means "unreported."
	ServingRevision string
}

// TargetMetrics contains resource metrics for a deployment
type TargetMetrics struct {
	CPUPercent    float64
	MemoryMB      float64
	RequestsCount int64
	Timestamp     time.Time
}

// CostEstimate contains a cost estimate for a deployment
type CostEstimate struct {
	ComputeMonthlyLow  float64
	ComputeMonthlyHigh float64
	InfrastructureMonthlyLow float64
	InfrastructureMonthlyHigh float64
	TotalMonthlyLow   float64
	TotalMonthlyHigh  float64
	UncertaintyNote   string
	PricingSource     string
	CalculatedAt      time.Time
}

// ActualCost contains actual incurred cost from cloud billing
type ActualCost struct {
	CostUSD          float64
	CostBreakdown    CostBreakdown
	StartDate        time.Time
	EndDate          time.Time
	Source           string // aws-cost-explorer, gcp-billing, azure-cost-management
}

// CostBreakdown contains cost broken down by category
type CostBreakdown struct {
	Compute        float64
	DataTransfer   float64
	Storage        float64
	Logging        float64
	LoadBalancer   float64
	Other          float64
}

// QuotaCheck contains the result of a quota check
type QuotaCheck struct {
	Available   bool
	Violations  []QuotaViolation
}

type QuotaViolation struct {
	Resource   string
	Requested  int
	Available  int
	Message    string
}

// CapabilityKey is the namespaced identifier for a provider capability.
// See phase3/provider-capability-matrix.md for the canonical set and
// their meaning. Adding a new key requires updating that doc, the
// provider capability table, and every existing provider's profile.
type CapabilityKey string

const (
	CapDeploy                CapabilityKey = "C-Deploy"
	CapDestroy               CapabilityKey = "C-Destroy"
	CapGetStatus             CapabilityKey = "C-GetStatus"
	CapRollback              CapabilityKey = "C-Rollback"
	CapTrafficSplit          CapabilityKey = "C-TrafficSplit"
	CapRevisionHistory       CapabilityKey = "C-RevisionHistory"
	CapCanaryRollout         CapabilityKey = "C-CanaryRollout"
	CapHealthReporting       CapabilityKey = "C-HealthReporting"
	CapDeploymentCancel      CapabilityKey = "C-DeploymentCancel"
	CapRevisionCleanup       CapabilityKey = "C-RevisionCleanup"
	CapDriftVisibility       CapabilityKey = "C-DriftVisibility"
	CapScalingIntrospection  CapabilityKey = "C-ScalingIntrospection"
	CapLogStreaming          CapabilityKey = "C-LogStreaming"
	CapMetricsVisibility     CapabilityKey = "C-MetricsVisibility"
	CapDestroyConfirmation   CapabilityKey = "C-DestroyConfirmation"
	CapIdempotentDeploy      CapabilityKey = "C-IdempotentDeploy"
	CapOperationTracking     CapabilityKey = "C-OperationTracking"
	CapEventualConsistency   CapabilityKey = "C-EventualConsistency"
)

// Capability is the honest semantic profile of a provider capability.
// A bare Supported boolean would be a lie: two providers that both say
// Supported=true for Rollback may have completely different operational
// semantics. Semantic + Constraints + UncertaintyP99 + Notes carry the
// truth that lets the reconciler and the UI behave correctly.
type Capability struct {
	// Supported is the coarse yes/no. Required.
	Supported bool

	// Semantic is the qualitative profile, e.g. "gradual-traffic-shift",
	// "instant-cutover", "not-found-poll", "status-deleted-poll".
	// Required when Supported=true. The reconciler's capability-aware
	// branches dispatch on this string, never on provider name.
	Semantic string

	// Constraints lists machine-readable conditions or limitations.
	// Free-form but stable; the UI may render them as tooltips.
	Constraints []string

	// UncertaintyP99 is the typical worst-case lag for eventually
	// consistent state. Used to tune the suspicion counter and the
	// destroy confirm window per-provider.
	UncertaintyP99 time.Duration

	// Notes is operator-readable explanation. Empty when not needed.
	Notes string
}

// CapabilitySet is the complete capability profile of a provider.
// Every CapabilityKey defined in this package MUST be present (with
// Supported=false and explanatory Notes if not implemented). Missing
// keys are an honesty bug, not a default-permissive case.
type CapabilitySet map[CapabilityKey]Capability

// AllCapabilityKeys lists the canonical capability key set in stable
// order. Used by tests and verification tooling to assert completeness
// of provider profiles. When adding a key, append here AND update every
// existing provider's profile.
var AllCapabilityKeys = []CapabilityKey{
	CapDeploy,
	CapDestroy,
	CapGetStatus,
	CapRollback,
	CapTrafficSplit,
	CapRevisionHistory,
	CapCanaryRollout,
	CapHealthReporting,
	CapDeploymentCancel,
	CapRevisionCleanup,
	CapDriftVisibility,
	CapScalingIntrospection,
	CapLogStreaming,
	CapMetricsVisibility,
	CapDestroyConfirmation,
	CapIdempotentDeploy,
	CapOperationTracking,
	CapEventualConsistency,
}

// Provider is the interface that all cloud provider implementations must satisfy
type Provider interface {
	// Capabilities returns the provider's complete capability profile.
	// The result is read-mostly; callers may cache it for the process
	// lifetime. Every key in AllCapabilityKeys MUST be present.
	//
	// See phase3/provider-capability-matrix.md and
	// phase3/provider-contract-evolution.md.
	Capabilities() CapabilitySet

	// ValidateCredentials tests that the cloud account credentials are valid
	// and have sufficient permissions for deployment
	ValidateCredentials(ctx context.Context, account *CloudAccount) error

	// Deploy creates or updates a cloud deployment
	Deploy(ctx context.Context, account *CloudAccount, spec *DeploySpec) (*DeployResult, error)

	// GetStatus retrieves the current status of a cloud deployment
	GetStatus(ctx context.Context, account *CloudAccount, target *DeploymentTarget) (*TargetStatus, error)

	// GetOperation retrieves the status of an in-flight operation
	// Returns nil if operation is not found or not tracked
	GetOperation(ctx context.Context, account *CloudAccount, operationID string) (*Operation, error)

	// Rollback rolls back a deployment to a previous revision
	// targetRevision can be empty to rollback to the previous revision
	Rollback(ctx context.Context, account *CloudAccount, target *DeploymentTarget, targetRevision string) (*DeployResult, error)

	// GetMetrics retrieves current resource metrics for a deployment
	GetMetrics(ctx context.Context, account *CloudAccount, target *DeploymentTarget) (*TargetMetrics, error)

	// StreamLogs streams logs from the deployment to the provided writer
	StreamLogs(ctx context.Context, account *CloudAccount, target *DeploymentTarget, writer io.Writer) error

	// EstimateCost provides a cost estimate before deployment
	EstimateCost(ctx context.Context, account *CloudAccount, spec *DeploySpec) (*CostEstimate, error)

	// GetActualCost retrieves actual cost from cloud billing APIs
	GetActualCost(ctx context.Context, account *CloudAccount, target *DeploymentTarget, startDate, endDate time.Time) (*ActualCost, error)

	// Destroy removes all cloud resources for a deployment
	// Returns nil if the resource is already deleted (idempotent)
	Destroy(ctx context.Context, account *CloudAccount, target *DeploymentTarget) error

	// ListRegions returns available regions for the provider
	ListRegions(ctx context.Context, account *CloudAccount) ([]string, error)

	// CheckQuotas verifies the account has sufficient quotas for the deployment
	CheckQuotas(ctx context.Context, account *CloudAccount, spec *DeploySpec) (*QuotaCheck, error)
}

// ProviderRegistry holds registered providers
var providers = make(map[string]Provider)

// RegisterProvider registers a cloud provider implementation
func RegisterProvider(name string, p Provider) {
	providers[name] = p
}

// GetProvider returns the provider for the given name
func GetProvider(name string) (Provider, error) {
	p, ok := providers[name]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return p, nil
}

// Provider names
const (
	ProviderAWSECS    = "aws-ecs"
	ProviderGCPCloudRun = "gcp-cloudrun"
	ProviderAzureACA  = "azure-aca"
	ProviderKubernetes = "kubernetes"
)

// ErrProviderNotFound is returned when a provider is not found
var ErrProviderNotFound = &ProviderError{Code: "PROVIDER_NOT_FOUND", Message: "Provider not found"}

// ErrNotImplemented is returned by provider methods whose behavior is not
// yet implemented. Callers MUST distinguish this from a successful call:
// returning a zero-valued result alongside nil error would lie about state.
var ErrNotImplemented = &ProviderError{Code: "NOT_IMPLEMENTED", Message: "Provider method not implemented"}

type ProviderError struct {
	Code    string
	Message string
}

func (e *ProviderError) Error() string {
	return e.Message
}