package topology

import (
	"fmt"
	"sort"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// BuildCrossProviderDependencies extracts dependencies between nodes on different providers.
//
// Only cross-provider edges produce a CrossProviderDependency record.
// Same-provider dependencies are captured in FailureDomains instead.
//
// Pure function: same plan → same dependencies (stable sort applied).
func BuildCrossProviderDependencies(plan *orchestrator.ExecutionPlan) []CrossProviderDependency {
	if plan == nil {
		return nil
	}

	nodeProviderIndex := buildNodeProviderIndexFromPlan(plan)
	var deps []CrossProviderDependency
	depID := 0

	for _, stage := range plan.Stages {
		for _, depStageID := range stage.Dependencies {
			depStage := findStageInPlan(plan, depStageID)
			if depStage == nil {
				continue
			}
			for _, fromNode := range stage.Nodes {
				for _, toNode := range depStage.Nodes {
					fromProv := nodeProviderIndex[fromNode.NodeID]
					toProv := nodeProviderIndex[toNode.NodeID]
					if fromProv == toProv {
						continue // same-provider dependencies are not cross-provider
					}
					depID++
					deps = append(deps, CrossProviderDependency{
						DependencyID:           fmt.Sprintf("xpd-%d", depID),
						FromNodeID:             fromNode.NodeID,
						FromProvider:           fromProv,
						ToNodeID:               toNode.NodeID,
						ToProvider:             toProv,
						DependencyType:         inferDependencyType(fromNode.NodeID, toNode.NodeID),
						WhyExists:              fmt.Sprintf("node %q on %q depends on node %q on %q via execution stage ordering", fromNode.NodeID, fromProv, toNode.NodeID, toProv),
						ProviderImplications:   crossProviderImplications(fromProv, toProv),
						BlastRadiusImplication: fmt.Sprintf("failure of %q (%s) propagates to %q (%s)", toNode.NodeID, toProv, fromNode.NodeID, fromProv),
						RollbackImplication:    fmt.Sprintf("roll back %q (%s) before %q (%s) to preserve dependency ordering", fromNode.NodeID, fromProv, toNode.NodeID, toProv),
						FailureDomainImpact:    fmt.Sprintf("cross-domain dependency: %q and %q are in different failure domains", fromProv, toProv),
					})
				}
			}
		}
	}

	sort.Slice(deps, func(i, j int) bool {
		if deps[i].FromNodeID != deps[j].FromNodeID {
			return deps[i].FromNodeID < deps[j].FromNodeID
		}
		return deps[i].ToNodeID < deps[j].ToNodeID
	})
	return deps
}

// ─── internal helpers ──────────────────────────────────────────────────────

func buildNodeProviderIndexFromPlan(plan *orchestrator.ExecutionPlan) map[string]string {
	index := make(map[string]string)
	for _, s := range plan.Stages {
		for _, n := range s.Nodes {
			index[n.NodeID] = n.Provider
		}
	}
	return index
}

func findStageInPlan(plan *orchestrator.ExecutionPlan, stageID string) *orchestrator.ExecutionStage {
	for i := range plan.Stages {
		if plan.Stages[i].StageID == stageID {
			return &plan.Stages[i]
		}
	}
	return nil
}

func inferDependencyType(fromNodeID, toNodeID string) string {
	return inferEdgeType(fromNodeID, toNodeID) // reuse edge-type inference
}

func crossProviderImplications(fromProvider, toProvider string) string {
	return fmt.Sprintf(
		"latency between %q and %q may exceed intra-provider latency; network egress costs apply; "+
			"outage of %q does not automatically trigger %q failure detection",
		fromProvider, toProvider, toProvider, fromProvider,
	)
}
