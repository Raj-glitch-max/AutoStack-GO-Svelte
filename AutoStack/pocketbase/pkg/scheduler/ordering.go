package scheduler

import (
	"sort"

	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

// riskOrder maps risk levels to numeric sort keys.
// Lower numeric value → scheduled first (lower risk first = safer progression).
var riskOrder = map[planner.RiskLevel]int{
	planner.RiskLevelLow:      0,
	planner.RiskLevelMedium:   1,
	planner.RiskLevelHigh:     2,
	planner.RiskLevelCritical: 3,
}

// SortQueueEntries sorts entries by the deterministic ordering:
//  1. DependencyDepth ascending (root stages first)
//  2. RiskLevel ascending (lower risk first)
//  3. StageOrder ascending (original plan order as tie-breaker)
//  4. StageID lexical ascending (final tie-breaker — no map iteration)
func SortQueueEntries(entries []QueueEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.DependencyDepth != b.DependencyDepth {
			return a.DependencyDepth < b.DependencyDepth
		}
		ra, rb := riskOrder[a.RiskLevel], riskOrder[b.RiskLevel]
		if ra != rb {
			return ra < rb
		}
		if a.StageOrder != b.StageOrder {
			return a.StageOrder < b.StageOrder
		}
		return a.StageID < b.StageID
	})
}

// ComputeDependencyDepths returns a map from stageID → depth in the dependency DAG.
// Depth 0 = no dependencies (root stage). Depth k = at least k prerequisite stages.
// The same plan always produces the same depth map (deterministic).
func ComputeDependencyDepths(stages []orchestrator.ExecutionStage) map[string]int {
	// Build adjacency: stageID → list of stageIDs it depends on.
	depMap := make(map[string][]string, len(stages))
	for _, s := range stages {
		depMap[s.StageID] = append([]string(nil), s.Dependencies...)
	}

	depth := make(map[string]int, len(stages))
	visited := make(map[string]bool, len(stages))

	var compute func(id string) int
	compute = func(id string) int {
		if visited[id] {
			return depth[id]
		}
		visited[id] = true
		max := 0
		for _, dep := range depMap[id] {
			if d := compute(dep) + 1; d > max {
				max = d
			}
		}
		depth[id] = max
		return max
	}

	// Stable sort of stage IDs before iterating to prevent map-order nondeterminism.
	ids := make([]string, 0, len(stages))
	for _, s := range stages {
		ids = append(ids, s.StageID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		compute(id)
	}

	return depth
}

// extractProviders returns a sorted, deduplicated list of providers from a stage's nodes.
func extractProviders(stage orchestrator.ExecutionStage) []string {
	seen := make(map[string]bool)
	for _, n := range stage.Nodes {
		if n.Provider != "" {
			seen[n.Provider] = true
		}
	}
	result := make([]string, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}

// hasProviderConflict returns true when two provider sets share any element.
func hasProviderConflict(a, b []string) bool {
	aSet := make(map[string]bool, len(a))
	for _, p := range a {
		aSet[p] = true
	}
	for _, p := range b {
		if aSet[p] {
			return true
		}
	}
	return false
}
