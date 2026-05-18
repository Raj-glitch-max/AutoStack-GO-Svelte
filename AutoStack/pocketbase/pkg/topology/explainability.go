package topology

// topologyLimitations documents invariant constraints on topology analysis.
func topologyLimitations() []string {
	return []string{
		"Topology graph is derived from static execution plan — live provider state is not reflected.",
		"Region information is not embedded in provider strings; cross-region dependencies require explicit annotation.",
		"Cycle detection operates at the node level; stage-level cycles are separately detected by the planner.",
		"Failure-domain risk is a static classification; actual SLA and historical failure rates are not modeled.",
		"Cross-provider latency and egress costs are not quantified — operator assessment required.",
		"Capability matrix reflects documented provider features; individual account/region restrictions are not checked.",
	}
}

// ExplainTopology produces a TopologyExplanation with full dependency rationale.
//
// Every dependency record explains:
//   - why it exists (business reason)
//   - provider implications (latency, egress, consistency)
//   - blast-radius implications (what fails if the dependency fails)
//   - rollback implications (ordering constraints)
//   - failure-domain implications (cross-domain vs same-domain)
//
// Pure function: same inputs → same output.
func ExplainTopology(graph TopologyGraph, deps []CrossProviderDependency) TopologyExplanation {
	return TopologyExplanation{
		ExecutionID:  graph.ExecutionID,
		Dependencies: append([]CrossProviderDependency(nil), deps...),
		Limitations:  topologyLimitations(),
	}
}
