package planner

import (
	"fmt"
	"strings"
)

// Phase 4.0 — Risk classification engine.
//
// Risk is classified deterministically from the combination of ActionType
// and service specification. The same inputs always produce the same risk
// level. There is no ML, no adaptive learning, no dynamic risk modeling.
//
// ─────────────────────────────────────────────────────────────────────────
// RISK TAXONOMY
// ─────────────────────────────────────────────────────────────────────────
//
//   LOW:      scaling changes, metadata-only updates, annotation changes
//   MEDIUM:   env var changes, compute adjustments, autoscaling policy,
//             rollout strategy changes, health check changes
//   HIGH:     secret reference changes, image source changes, network
//             interface changes, IAM/credential changes, provider config
//             changes that may affect security identity
//   CRITICAL: destructive deletes, irreversible state mutations (no restore
//             API), dependency-collapse deletes (deleting a node that other
//             nodes depend on without first migrating them)

// ClassifyActionRisk classifies the risk of applying an action to a node,
// given the service specification that drives it.
func ClassifyActionRisk(action ActionType, svc ServiceSpec) RiskLevel {
	switch action {
	case ActionNoOp:
		return RiskLevelLow

	case ActionCreate:
		// Creating a new service is medium by default. High if it exposes
		// network interfaces or binds to secrets.
		if len(svc.SecretRefs) > 0 || len(svc.EnvVars) > 0 && hasSecretEnvVars(svc.EnvVars) {
			return RiskLevelHigh
		}
		return RiskLevelMedium

	case ActionUpdate:
		return classifyUpdateRisk(svc)

	case ActionDelete:
		// All deletes are at minimum High. Provider deletes are rarely
		// reversible without a backup/restore pipeline.
		return RiskLevelCritical

	case ActionRollback:
		return RiskLevelHigh

	case ActionSimulate:
		return RiskLevelLow

	default:
		return RiskLevelMedium
	}
}

// classifyUpdateRisk evaluates what changed in an update action.
// Since the planner operates on DesiredState (not on a delta), this
// function uses the SERVICE SPEC CONTENT as a proxy for what fields
// could be sensitive. A future version that accepts old+new specs
// can use DiffDeploySpec for more precise classification.
func classifyUpdateRisk(svc ServiceSpec) RiskLevel {
	// Secret references → always High.
	if len(svc.SecretRefs) > 0 {
		return RiskLevelHigh
	}
	// Sensitive env var names → High.
	if hasSecretEnvVars(svc.EnvVars) {
		return RiskLevelHigh
	}
	// Network interfaces with TLS or explicit ingress → High.
	// (captured via service annotations for now since ServiceSpec
	// doesn't embed NetworkSpec directly in DesiredState)
	if isNetworkSensitive(svc) {
		return RiskLevelHigh
	}
	// Compute / scale / env / health changes → Medium.
	if svc.Compute.MemoryLimitMB > 0 || svc.Compute.CPULimitVCPU > 0 ||
		svc.Scale.MaxReplicas > 0 || len(svc.EnvVars) > 0 || svc.HealthCheck != nil {
		return RiskLevelMedium
	}
	// Annotation-only or metadata → Low.
	return RiskLevelLow
}

// ClassifyDeleteRisk classifies the risk of deleting a node, considering
// whether other nodes depend on it.
func ClassifyDeleteRisk(nodeID string, g *PlanGraph) RiskLevel {
	node, ok := g.NodeIndex[nodeID]
	if !ok {
		return RiskLevelHigh
	}
	if len(node.ReverseDependencies) > 0 {
		// Deleting a node that others depend on collapses those dependencies.
		// This is critical because the blast radius extends beyond the deleted node.
		return RiskLevelCritical
	}
	return RiskLevelCritical // all deletes are at minimum Critical
}

// BlastRadius estimates the number of nodes affected if nodeID fails
// during plan execution. Includes the node itself + all transitive dependents.
func BlastRadius(nodeID string, g *PlanGraph) (affectedNodes []string, radius int) {
	visited := make(map[string]bool)
	var walk func(id string)
	walk = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		affectedNodes = append(affectedNodes, id)
		node, ok := g.NodeIndex[id]
		if !ok {
			return
		}
		for _, rev := range node.ReverseDependencies {
			walk(rev)
		}
	}
	walk(nodeID)

	// Sort for determinism.
	sortedStrings(affectedNodes)
	return affectedNodes, len(affectedNodes)
}

// ─────────────────────────────────────────────────────────────────────────
// NODE RISK HELPERS (used by BuildPlanGraph)
// ─────────────────────────────────────────────────────────────────────────

func classifyNodeRisk(action ActionType, svc ServiceSpec) RiskLevel {
	return ClassifyActionRisk(action, svc)
}

func whyNodeRisk(action ActionType, svc ServiceSpec) string {
	level := ClassifyActionRisk(action, svc)
	switch level {
	case RiskLevelCritical:
		return fmt.Sprintf("action=%s is destructive; provider-side restore is not guaranteed", action)
	case RiskLevelHigh:
		reasons := []string{}
		if len(svc.SecretRefs) > 0 {
			reasons = append(reasons, "secret references present")
		}
		if hasSecretEnvVars(svc.EnvVars) {
			reasons = append(reasons, "sensitive env var names detected")
		}
		if isNetworkSensitive(svc) {
			reasons = append(reasons, "network/security configuration present")
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "action type defaults to high risk")
		}
		return strings.Join(reasons, "; ")
	case RiskLevelMedium:
		return fmt.Sprintf("action=%s affects compute, scale, env, or health check config", action)
	default:
		return fmt.Sprintf("action=%s produces no provider-visible side effects", action)
	}
}

func hasSecretEnvVars(vars []EnvVarIntent) bool {
	for _, v := range vars {
		upper := strings.ToUpper(v.Name)
		for _, kw := range []string{"SECRET", "PASSWORD", "TOKEN", "KEY", "CREDENTIAL", "PASS", "AUTH"} {
			if strings.Contains(upper, kw) {
				return true
			}
		}
	}
	return false
}

func isNetworkSensitive(svc ServiceSpec) bool {
	// In the current model, network sensitivity is inferred from annotations.
	// A future version should embed NetworkSpec in ServiceSpec.
	for k, v := range svc.Annotations {
		k, v = strings.ToLower(k), strings.ToLower(v)
		if strings.Contains(k, "network") || strings.Contains(k, "ingress") ||
			strings.Contains(k, "tls") || strings.Contains(v, "loadbalancer") {
			return true
		}
	}
	return false
}

func sortedStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
