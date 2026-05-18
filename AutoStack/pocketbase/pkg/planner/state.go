package planner

// Phase 4.0 — DesiredState model.
//
// DesiredState is an immutable, deterministic snapshot of what the operator
// intends the infrastructure to look like. It is the single input to the
// planning engine. Every planning artifact (PlanGraph, ChangeSet, simulation)
// is derived deterministically from a DesiredState.
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN INVARIANTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. IMMUTABLE: once constructed, a DesiredState is never mutated.
//      The planning engine creates new DesiredState snapshots; it never
//      modifies existing ones. This enables safe concurrent reads and
//      deterministic plan replay.
//
//   2. DETERMINISTIC: ComputeDesiredStateHash(s) returns byte-identical
//      output for structurally identical states, regardless of construction
//      order. This requires stable ordering of all slice fields.
//
//   3. REPLAY-SAFE: a DesiredState serialized to JSON and later deserialized
//      produces the same hash as the original. No ephemeral fields.
//
//   4. HONEST SCOPE: DesiredState describes INTENT, not execution. It does
//      not contain operational status, retry state, or provider truth. Those
//      live in the reconciler layer (Phase 3) and are passed to the planner
//      as evidence references, not embedded state.
//
// ─────────────────────────────────────────────────────────────────────────
// PHASE 4 FORBIDDEN BEHAVIORS
// ─────────────────────────────────────────────────────────────────────────
//
//   - DesiredState NEVER mutates infrastructure.
//   - DesiredState NEVER calls a provider API.
//   - DesiredState NEVER contains runtime credentials.
//   - DesiredState NEVER auto-heals or auto-corrects.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

// DesiredStateSchemaVersion identifies the DesiredState JSON shape.
// Increment when fields are added or removed.
const DesiredStateSchemaVersion = "4.0.0"

// ─────────────────────────────────────────────────────────────────────────
// CORE TYPES
// ─────────────────────────────────────────────────────────────────────────

// EvidenceRef is a pointer to a persisted record that justifies a planning
// decision. The planner never reads the referenced record itself — it
// passes the ref as metadata so operators can trace decisions back to
// forensic evidence in the reconciler layer.
type EvidenceRef struct {
	Collection string `json:"collection"` // e.g. "provider_observations", "incidents"
	ID         string `json:"id"`
	Annotation string `json:"annotation,omitempty"` // human-readable hint
}

// Environment describes the target deployment environment. Stable string;
// persisted in plan output.
type Environment string

const (
	EnvironmentProduction  Environment = "production"
	EnvironmentStaging     Environment = "staging"
	EnvironmentDevelopment Environment = "development"
	EnvironmentDRDrill     Environment = "dr-drill"
)

// ServiceSpec describes one service to be deployed or maintained.
type ServiceSpec struct {
	// ID is a stable, unique identifier for this service within the DesiredState.
	// It becomes the NodeID in the PlanGraph.
	ID string `json:"id"`

	// Name is a human-readable label. May be non-unique; ID is the key.
	Name string `json:"name"`

	// Provider is the cloud provider that hosts this service.
	// Must match a registered provider (e.g. "aws-ecs", "cloud-run").
	Provider string `json:"provider"`

	// Region is the provider region for deployment.
	Region string `json:"region"`

	// Image is the container image specification.
	Image ImageIntent `json:"image"`

	// Compute declares CPU and memory requirements.
	Compute ComputeIntent `json:"compute"`

	// Scale declares replica bounds and autoscaling policy.
	Scale ScaleIntent `json:"scale"`

	// EnvVars lists environment variables. Sorted by Name for determinism.
	EnvVars []EnvVarIntent `json:"env_vars"`

	// SecretRefs lists secret bindings. Sorted by Name for determinism.
	SecretRefs []SecretRefIntent `json:"secret_refs"`

	// HealthCheck describes the health probe configuration.
	HealthCheck *HealthCheckIntent `json:"health_check,omitempty"`

	// DependsOn is a list of ServiceSpec IDs this service requires.
	// Sorted for determinism.
	DependsOn []string `json:"depends_on"`

	// Annotations are non-runtime key-value metadata (labels, cost tags).
	// Sorted by key for determinism.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ImageIntent declares the container image to deploy. This is INTENT, not
// a guarantee of what will actually be pulled — provider-side caching and
// tag mutation are not in scope.
//
// Honest limitation: no digest field. Tag equality is the only identity
// check. Same-tag/different-digest drift is not detectable by the planner.
type ImageIntent struct {
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	PullPolicy string `json:"pull_policy"`
}

// ComputeIntent declares CPU and memory requirements.
type ComputeIntent struct {
	CPURequestVCPU  float64 `json:"cpu_request_vcpu"`
	CPULimitVCPU    float64 `json:"cpu_limit_vcpu"`
	MemoryRequestMB int     `json:"memory_request_mb"`
	MemoryLimitMB   int     `json:"memory_limit_mb"`
}

// ScaleIntent declares autoscaling policy.
type ScaleIntent struct {
	MinReplicas        int    `json:"min_replicas"`
	MaxReplicas        int    `json:"max_replicas"`
	ScaleMetric        string `json:"scale_metric"`
	ScaleTargetPercent int    `json:"scale_target_percent"`
}

// EnvVarIntent is a single environment variable in the desired state.
// Values that appear sensitive are safe to persist here because
// DesiredState is planner-internal and never logged.
type EnvVarIntent struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SecretRefIntent is a reference to an external secret, not the secret
// itself. The planner never resolves or stores actual secret values.
type SecretRefIntent struct {
	Name       string `json:"name"`
	Source     string `json:"source"` // e.g. "aws-secrets-manager"
	SecretName string `json:"secret_name"`
	SecretKey  string `json:"secret_key"`
}

// HealthCheckIntent describes health probe configuration.
type HealthCheckIntent struct {
	LivenessPath        string `json:"liveness_path"`
	LivenessPort        int    `json:"liveness_port"`
	ReadinessPath       string `json:"readiness_path"`
	ReadinessPort       int    `json:"readiness_port"`
	InitialDelaySeconds int    `json:"initial_delay_seconds"`
	PeriodSeconds       int    `json:"period_seconds"`
	FailureThreshold    int    `json:"failure_threshold"`
}

// DependencySpec declares an external dependency (database, queue, etc.)
// that services may reference. The planner models these as non-service
// PlanNodes so the dependency graph is complete.
type DependencySpec struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // database, queue, cache, storage
	Provider string `json:"provider"`
	Name     string `json:"name"`
	// RequiredBy lists service IDs that depend on this resource.
	RequiredBy []string `json:"required_by"`
}

// PolicySpec is a named policy that governs planner behavior.
// Policies are references: the planner uses their IDs as tags on decisions
// but does not evaluate arbitrary policy code.
type PolicySpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"` // safety, cost, compliance, rollout
}

// ConstraintSpec expresses a planning constraint. Constraints can block or
// warn on plan generation but do not automatically enforce themselves at
// the infrastructure level.
type ConstraintSpec struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // block | warn
	Applies     string `json:"applies"`  // target: "all" | specific node ID
}

// ProviderBinding associates a service with a cloud account credential reference.
// The planner records the account reference but NEVER resolves or uses credentials.
type ProviderBinding struct {
	ServiceID string `json:"service_id"`
	AccountID string `json:"account_id"` // PocketBase ID; never the credential itself
	Provider  string `json:"provider"`
	Region    string `json:"region"`
}

// RolloutStrategySpec declares how the planner should order mutations.
type RolloutStrategySpec struct {
	Mode              string `json:"mode"`               // rolling | blue-green | canary | all-at-once
	MaxSurge          int    `json:"max_surge"`           // for rolling/canary
	MaxUnavailable    int    `json:"max_unavailable"`     // for rolling
	StepPercent       int    `json:"step_percent"`        // for canary
	RequireApproval   bool   `json:"require_approval"`    // gate every stage
	PauseOnHighRisk   bool   `json:"pause_on_high_risk"`
}

// SafetyProfile governs what the planner is allowed to plan.
// DryRunOnly = true means the planner will never emit a ChangeSet that
// could be executed. Simulation output is the only product.
type SafetyProfile struct {
	DryRunOnly              bool     `json:"dry_run_only"`
	MaxBlastRadiusNodes     int      `json:"max_blast_radius_nodes"`
	ForbidDestructiveDeletes bool    `json:"forbid_destructive_deletes"`
	RequireApprovalAbove    string   `json:"require_approval_above"` // risk level: low|medium|high|critical
	AllowedProviders        []string `json:"allowed_providers"`
}

// ─────────────────────────────────────────────────────────────────────────
// DESIRED STATE
// ─────────────────────────────────────────────────────────────────────────

// DesiredState is the immutable planning input. Construct with
// NewDesiredState() to ensure deterministic field ordering and hash
// computation.
type DesiredState struct {
	SchemaVersion string `json:"schema_version"`

	// StateVersion is a monotonic sequence number for this DesiredState.
	// Callers must increment it when creating a new state from an old one.
	StateVersion int `json:"state_version"`

	GeneratedAt string      `json:"generated_at"` // UTC RFC3339
	Environment Environment `json:"environment"`

	// Services is sorted by Service.ID for determinism.
	Services []ServiceSpec `json:"services"`

	// Dependencies is sorted by DependencySpec.ID for determinism.
	Dependencies []DependencySpec `json:"dependencies"`

	// Policies is sorted by PolicySpec.ID.
	Policies []PolicySpec `json:"policies"`

	// Constraints is sorted by ConstraintSpec.ID.
	Constraints []ConstraintSpec `json:"constraints"`

	// ProviderBindings is sorted by ServiceID then AccountID.
	ProviderBindings []ProviderBinding `json:"provider_bindings"`

	RolloutStrategy RolloutStrategySpec `json:"rollout_strategy"`
	SafetyProfile   SafetyProfile       `json:"safety_profile"`

	// EvidenceRefs are pointers to reconciler-layer records that informed
	// the construction of this DesiredState (e.g. a drift record that
	// triggered a replica count change).
	EvidenceRefs []EvidenceRef `json:"evidence_refs"`

	// DesiredStateHash is the SHA-256 of the canonical JSON of this struct
	// with DesiredStateHash set to "". Computed by NewDesiredState.
	DesiredStateHash string `json:"desired_state_hash"`
}

// NewDesiredState constructs and validates a DesiredState. It normalizes
// all slice ordering for determinism and computes the hash.
func NewDesiredState(
	version int,
	env Environment,
	services []ServiceSpec,
	deps []DependencySpec,
	policies []PolicySpec,
	constraints []ConstraintSpec,
	bindings []ProviderBinding,
	strategy RolloutStrategySpec,
	safety SafetyProfile,
	evidence []EvidenceRef,
) *DesiredState {
	s := &DesiredState{
		SchemaVersion:    DesiredStateSchemaVersion,
		StateVersion:     version,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Environment:      env,
		Services:         normalizeServices(services),
		Dependencies:     normalizeDeps(deps),
		Policies:         normalizePolicies(policies),
		Constraints:      normalizeConstraints(constraints),
		ProviderBindings: normalizeBindings(bindings),
		RolloutStrategy:  strategy,
		SafetyProfile:    safety,
		EvidenceRefs:     evidence,
		DesiredStateHash: "",
	}
	s.DesiredStateHash = ComputeDesiredStateHash(s)
	return s
}

// ComputeDesiredStateHash returns the SHA-256 hex of the canonical JSON of
// s with DesiredStateHash set to "". Deterministic for identical inputs.
func ComputeDesiredStateHash(s *DesiredState) string {
	// Work on a copy so we don't mutate the original.
	copy := *s
	copy.DesiredStateHash = ""
	// GeneratedAt changes per call — exclude it from the hash so the hash
	// reflects CONTENT identity, not creation-time identity. This means two
	// states with the same content but different GeneratedAt have the same hash.
	copy.GeneratedAt = ""
	payload, err := json.Marshal(copy)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// ─────────────────────────────────────────────────────────────────────────
// NORMALIZATION HELPERS
// ─────────────────────────────────────────────────────────────────────────

func normalizeServices(ss []ServiceSpec) []ServiceSpec {
	for i := range ss {
		sort.Strings(ss[i].DependsOn)
		sort.Slice(ss[i].EnvVars, func(a, b int) bool {
			return ss[i].EnvVars[a].Name < ss[i].EnvVars[b].Name
		})
		sort.Slice(ss[i].SecretRefs, func(a, b int) bool {
			return ss[i].SecretRefs[a].Name < ss[i].SecretRefs[b].Name
		})
	}
	out := make([]ServiceSpec, len(ss))
	copy(out, ss)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func normalizeDeps(ds []DependencySpec) []DependencySpec {
	out := make([]DependencySpec, len(ds))
	copy(out, ds)
	for i := range out {
		sort.Strings(out[i].RequiredBy)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func normalizePolicies(ps []PolicySpec) []PolicySpec {
	out := make([]PolicySpec, len(ps))
	copy(out, ps)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func normalizeConstraints(cs []ConstraintSpec) []ConstraintSpec {
	out := make([]ConstraintSpec, len(cs))
	copy(out, cs)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func normalizeBindings(bs []ProviderBinding) []ProviderBinding {
	out := make([]ProviderBinding, len(bs))
	copy(out, bs)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ServiceID != out[j].ServiceID {
			return out[i].ServiceID < out[j].ServiceID
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}
