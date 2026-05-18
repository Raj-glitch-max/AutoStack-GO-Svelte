// Package topology models cross-provider dependency and failure-domain semantics.
//
// Core invariants:
//   - All functions are pure: same inputs → same output.
//   - Dependency cycles are forbidden and explicitly detected.
//   - Every dependency explains why it exists and its blast-radius implications.
//   - No failover automation, no HA coordination, no distributed orchestration.
package topology

import "github.com/janlauber/one-click/pkg/orchestrator"

const TopologySnapshotSchemaVersion = "4.6.0"

// ─── Provider node and graph ───────────────────────────────────────────────

// ProviderNode represents one provider instance in the topology.
type ProviderNode struct {
	// ProviderID is a stable unique identifier: "<provider>/<region>".
	ProviderID string `json:"provider_id"`
	// Provider is the normalized provider string ("aws-ecs", "gcp-cloudrun", etc.).
	Provider string `json:"provider"`
	// Region is the deployment region for this provider instance.
	Region string `json:"region"`
	// NodeIDs lists the execution-plan node IDs deployed on this provider.
	NodeIDs []string `json:"node_ids"`
}

// TopologyEdge is a directed dependency between two execution-plan nodes.
type TopologyEdge struct {
	// FromNodeID is the dependent node (the one that needs ToNodeID).
	FromNodeID string `json:"from_node_id"`
	// ToNodeID is the dependency (the one that must exist first).
	ToNodeID      string `json:"to_node_id"`
	EdgeType      string `json:"edge_type"` // "ingress-to-backend","service-to-db","cache-to-api","queue-to-service"
	CrossProvider bool   `json:"cross_provider"`
	// Explanation documents why this edge exists.
	Explanation string `json:"explanation"`
	// BlastRadiusImplication describes what happens to FromNodeID if ToNodeID fails.
	BlastRadiusImplication string `json:"blast_radius_implication"`
	// RollbackImplication describes rollback ordering constraints.
	RollbackImplication string `json:"rollback_implication"`
	// FailureDomainImplication describes failure-domain membership.
	FailureDomainImplication string `json:"failure_domain_implication"`
}

// TopologyGraph is the complete graph of providers and dependencies for an execution.
type TopologyGraph struct {
	ExecutionID string         `json:"execution_id"`
	Nodes       []ProviderNode `json:"nodes"`
	Edges       []TopologyEdge `json:"topology_edges"`
	// CycleDetected is true if the dependency graph contains a cycle.
	CycleDetected bool `json:"cycle_detected"`
	// CyclePaths lists the detected cycles (stageID paths).
	CyclePaths [][]string `json:"cycle_paths,omitempty"`
	CreatedAt  string     `json:"created_at"`
}

// ─── Provider capabilities ─────────────────────────────────────────────────

// ProviderCapabilities describes what operations a provider supports.
// Capabilities are deterministic constants — they do not require a live API call.
type ProviderCapabilities struct {
	Provider              string   `json:"provider"`
	SupportsCancellation  bool     `json:"supports_cancellation"`
	SupportsRollback      bool     `json:"supports_rollback"`
	SupportsHealthProbe   bool     `json:"supports_health_probe"`
	SupportsAsyncRollout  bool     `json:"supports_async_rollout"`
	SupportsTrafficShift  bool     `json:"supports_traffic_shift"`
	SupportsStagedVerify  bool     `json:"supports_staged_verify"`
	Limitations           []string `json:"limitations"`
}

// ─── Failure domains ───────────────────────────────────────────────────────

// FailureDomain is a set of nodes that are expected to fail together.
// A provider outage or region outage defines a domain.
type FailureDomain struct {
	DomainID string `json:"domain_id"`
	Provider string `json:"provider"`
	Region   string `json:"region"`
	// NodeIDs lists execution-plan node IDs within this domain.
	NodeIDs []string `json:"node_ids"`
	// OutageRisk qualifies the probability of a correlated failure.
	OutageRisk string `json:"outage_risk"` // "high","medium","low"
	// Explanation describes what would cause all nodes in this domain to fail.
	Explanation string `json:"explanation"`
}

// ─── Cross-provider dependencies ──────────────────────────────────────────

// CrossProviderDependency models a dependency between nodes on different providers.
// Every field is mandatory — no implicit semantics.
type CrossProviderDependency struct {
	DependencyID string `json:"dependency_id"`
	FromNodeID   string `json:"from_node_id"`
	FromProvider string `json:"from_provider"`
	ToNodeID     string `json:"to_node_id"`
	ToProvider   string `json:"to_provider"`
	// DependencyType classifies the relationship.
	DependencyType string `json:"dependency_type"` // "ingress","backend","database","queue","cache","storage"
	// WhyExists documents the business reason for this dependency.
	WhyExists string `json:"why_exists"`
	// ProviderImplications describes provider-specific constraints.
	ProviderImplications string `json:"provider_implications"`
	// BlastRadiusImplication describes what the dependency failure causes.
	BlastRadiusImplication string `json:"blast_radius_implication"`
	// RollbackImplication describes the required rollback ordering.
	RollbackImplication string `json:"rollback_implication"`
	// FailureDomainImpact describes impact on failure-domain partitioning.
	FailureDomainImpact string `json:"failure_domain_impact"`
}

// ─── Explainability ────────────────────────────────────────────────────────

// TopologyExplanation documents all dependencies with full rationale.
type TopologyExplanation struct {
	ExecutionID  string                    `json:"execution_id"`
	Dependencies []CrossProviderDependency `json:"dependencies"`
	Limitations  []string                  `json:"limitations"`
}

// ─── Snapshot ──────────────────────────────────────────────────────────────

// TopologySnapshot is a tamper-evident export of the topology state.
// GeneratedAt and TopologyHash are excluded from the hash.
type TopologySnapshot struct {
	SchemaVersion  string                    `json:"schema_version"`
	ExecutionID    string                    `json:"execution_id"`
	Graph          TopologyGraph             `json:"graph"`
	Capabilities   []ProviderCapabilities    `json:"capabilities"`
	FailureDomains []FailureDomain           `json:"failure_domains"`
	Dependencies   []CrossProviderDependency `json:"dependencies"`
	Limitations    []string                  `json:"limitations"`
	GeneratedAt    string                    `json:"generated_at"`
	TopologyHash   string                    `json:"topology_hash"`
}

// ─── internal bundle ───────────────────────────────────────────────────────

// topoInput bundles a plan's stages for internal traversal.
type topoInput struct {
	stages    []orchestrator.ExecutionStage
	nodeIndex map[string]orchestrator.StageNode // nodeID → StageNode
}
