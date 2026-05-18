package scheduler

import (
	"fmt"
	"sort"
)

// BuildConcurrencyGroups derives which stages from the queue can execute in parallel.
//
// Two stages are concurrent-safe if ALL of the following hold:
//  1. They share the same DependencyDepth (neither can depend on the other).
//  2. Their provider sets do not overlap (no provider conflict).
//  3. Their BlastRadius sum does not exceed the safe threshold (≤ 5).
//
// Pure function: same queue → same groups. Groups are sorted by GroupID for stability.
func BuildConcurrencyGroups(queue StageQueue) []ConcurrencyGroup {
	if len(queue.Entries) == 0 {
		return nil
	}

	// Group entries by dependency depth.
	byDepth := make(map[int][]QueueEntry)
	for _, e := range queue.Entries {
		byDepth[e.DependencyDepth] = append(byDepth[e.DependencyDepth], e)
	}

	// Sort depths for stable iteration.
	depths := make([]int, 0, len(byDepth))
	for d := range byDepth {
		depths = append(depths, d)
	}
	sort.Ints(depths)

	var groups []ConcurrencyGroup
	groupIndex := 0

	for _, depth := range depths {
		entries := byDepth[depth]
		// Only one entry at this depth — cannot be concurrent.
		if len(entries) == 1 {
			continue
		}

		// Greedy grouping: build groups of provider-conflict-free entries.
		// Entries are already in deterministic order from the queue.
		assigned := make([]bool, len(entries))
		for i := 0; i < len(entries); i++ {
			if assigned[i] {
				continue
			}
			group := []QueueEntry{entries[i]}
			assigned[i] = true
			combinedProviders := append([]string(nil), entries[i].Providers...)
			blastSum := entries[i].BlastRadius

			for j := i + 1; j < len(entries); j++ {
				if assigned[j] {
					continue
				}
				candidate := entries[j]
				if hasProviderConflict(combinedProviders, candidate.Providers) {
					continue
				}
				if blastSum+candidate.BlastRadius > 5 {
					continue
				}
				group = append(group, candidate)
				assigned[j] = true
				combinedProviders = mergeSorted(combinedProviders, candidate.Providers)
				blastSum += candidate.BlastRadius
			}

			if len(group) > 1 {
				stageIDs := make([]string, len(group))
				for k, g := range group {
					stageIDs[k] = g.StageID
				}
				sort.Strings(stageIDs)
				groups = append(groups, ConcurrencyGroup{
					GroupID:   fmt.Sprintf("cg-%d-%d", depth, groupIndex),
					StageIDs:  stageIDs,
					Rationale: fmt.Sprintf("depth=%d stages share no provider conflict and combined blast radius ≤5", depth),
				})
				groupIndex++
			}
		}
	}

	return groups
}

// ConflictsWithGroup returns true when stageID appears in any concurrency group
// alongside another stage that would create a provider or dependency conflict.
func ConflictsWithGroup(groups []ConcurrencyGroup, stageID string) bool {
	for _, g := range groups {
		for _, id := range g.StageIDs {
			if id == stageID {
				return false // in a group = cleared for concurrency
			}
		}
	}
	return false
}

// mergeSorted merges two sorted slices and deduplicates.
func mergeSorted(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		seen[v] = true
	}
	result := make([]string, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}
