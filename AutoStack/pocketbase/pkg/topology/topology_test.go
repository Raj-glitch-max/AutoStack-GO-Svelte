package topology

import (
	"testing"

	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func makeNode(id, provider string) orchestrator.StageNode {
	return orchestrator.StageNode{NodeID: id, Provider: provider, Action: planner.ActionCreate}
}

func makeStage(id string, order int, deps []string, nodes ...orchestrator.StageNode) orchestrator.ExecutionStage {
	return orchestrator.ExecutionStage{
		StageID:      id,
		StageOrder:   order,
		Dependencies: deps,
		Nodes:        nodes,
	}
}

func makePlan(stages ...orchestrator.ExecutionStage) *orchestrator.ExecutionPlan {
	return &orchestrator.ExecutionPlan{
		ExecutionPlanHash: "test-hash",
		Stages:            stages,
	}
}

// ─── graph ─────────────────────────────────────────────────────────────────

func TestBuildTopologyGraph_NilPlan(t *testing.T) {
	g := BuildTopologyGraph(nil)
	if len(g.Nodes) != 0 {
		t.Fatal("nil plan must produce empty graph")
	}
}

func TestBuildTopologyGraph_SingleStage(t *testing.T) {
	plan := makePlan(makeStage("s0", 0, nil, makeNode("node-a", "aws-ecs")))
	g := BuildTopologyGraph(plan)
	if len(g.Nodes) != 1 {
		t.Fatalf("single provider must produce 1 provider node, got %d", len(g.Nodes))
	}
}

func TestBuildTopologyGraph_TwoProvidersOneEdge(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("backend", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("frontend", "gcp-cloudrun")),
	)
	g := BuildTopologyGraph(plan)
	if len(g.Nodes) != 2 {
		t.Fatalf("two providers must produce 2 provider nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) == 0 {
		t.Fatal("stage dependency must produce at least one topology edge")
	}
}

func TestBuildTopologyGraph_CrossProviderEdge(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("db", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("api", "gcp-cloudrun")),
	)
	g := BuildTopologyGraph(plan)
	for _, e := range g.Edges {
		if e.CrossProvider {
			return // found one
		}
	}
	t.Fatal("dependency across different providers must produce a CrossProvider edge")
}

func TestBuildTopologyGraph_Deterministic(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("a", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("b", "gcp-cloudrun")),
	)
	g1 := BuildTopologyGraph(plan)
	g2 := BuildTopologyGraph(plan)
	if len(g1.Nodes) != len(g2.Nodes) {
		t.Fatal("same plan must produce same node count")
	}
	for i := range g1.Nodes {
		if g1.Nodes[i].ProviderID != g2.Nodes[i].ProviderID {
			t.Fatalf("node order must be deterministic at index %d", i)
		}
	}
}

func TestDetectTopologyCycles_NoCycle(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("a", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("b", "aws-ecs")),
	)
	g := BuildTopologyGraph(plan)
	cycles := DetectTopologyCycles(g)
	if len(cycles) != 0 {
		t.Fatalf("acyclic graph must have 0 cycles, got %d", len(cycles))
	}
	if g.CycleDetected {
		t.Fatal("CycleDetected must be false for acyclic graph")
	}
}

func TestBuildTopologyGraph_EdgeExplanationNonEmpty(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("db", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("api", "gcp-cloudrun")),
	)
	g := BuildTopologyGraph(plan)
	for _, e := range g.Edges {
		if e.Explanation == "" {
			t.Fatal("every edge must have a non-empty Explanation")
		}
		if e.BlastRadiusImplication == "" {
			t.Fatal("every edge must have a non-empty BlastRadiusImplication")
		}
	}
}

// ─── capabilities ──────────────────────────────────────────────────────────

func TestCapabilitiesForProvider_KnownProvider(t *testing.T) {
	caps := CapabilitiesForProvider("gcp-cloudrun")
	if caps.Provider != "gcp-cloudrun" {
		t.Fatalf("provider mismatch: %q", caps.Provider)
	}
	if !caps.SupportsTrafficShift {
		t.Fatal("Cloud Run must support traffic shifting")
	}
}

func TestCapabilitiesForProvider_UnknownProvider(t *testing.T) {
	caps := CapabilitiesForProvider("unknown-provider")
	if caps.SupportsCancellation {
		t.Fatal("unknown provider must default to false for all capabilities")
	}
	if len(caps.Limitations) == 0 {
		t.Fatal("unknown provider must have non-empty limitations")
	}
}

func TestBuildCapabilityMatrix_Deterministic(t *testing.T) {
	providers := []string{"gcp-cloudrun", "aws-ecs", "aws-ecs"} // duplicate
	caps := BuildCapabilityMatrix(providers)
	if len(caps) != 2 {
		t.Fatalf("BuildCapabilityMatrix must deduplicate: expected 2, got %d", len(caps))
	}
	// Must be sorted.
	if caps[0].Provider >= caps[1].Provider {
		t.Fatal("capability matrix must be sorted by provider name")
	}
}

func TestCapabilities_AzureACA_NoRollback(t *testing.T) {
	caps := CapabilitiesForProvider("azure-aca")
	if caps.SupportsRollback {
		t.Fatal("Azure ACA does not support first-class rollback")
	}
}

func TestProvidersFromGraph(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("a", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("b", "gcp-cloudrun")),
	)
	g := BuildTopologyGraph(plan)
	providers := ProvidersFromGraph(g)
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
}

// ─── failure domains ───────────────────────────────────────────────────────

func TestBuildFailureDomains_SingleProvider(t *testing.T) {
	plan := makePlan(makeStage("s0", 0, nil,
		makeNode("a", "aws-ecs"),
		makeNode("b", "aws-ecs"),
	))
	g := BuildTopologyGraph(plan)
	domains := BuildFailureDomains(g)
	if len(domains) != 1 {
		t.Fatalf("single provider must produce 1 failure domain, got %d", len(domains))
	}
}

func TestBuildFailureDomains_TwoProviders(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("a", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("b", "gcp-cloudrun")),
	)
	g := BuildTopologyGraph(plan)
	domains := BuildFailureDomains(g)
	if len(domains) != 2 {
		t.Fatalf("two providers must produce 2 failure domains, got %d", len(domains))
	}
}

func TestBuildFailureDomains_DomainIDNonEmpty(t *testing.T) {
	plan := makePlan(makeStage("s0", 0, nil, makeNode("a", "aws-ecs")))
	g := BuildTopologyGraph(plan)
	domains := BuildFailureDomains(g)
	for _, d := range domains {
		if d.DomainID == "" {
			t.Fatal("DomainID must be non-empty")
		}
		if d.Explanation == "" {
			t.Fatal("Explanation must be non-empty")
		}
	}
}

func TestCrossDomainEdges(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("db", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("api", "gcp-cloudrun")),
	)
	g := BuildTopologyGraph(plan)
	crossEdges := CrossDomainEdges(g)
	if len(crossEdges) == 0 {
		t.Fatal("cross-provider dependency must produce cross-domain edges")
	}
}

// ─── cross-provider dependencies ───────────────────────────────────────────

func TestBuildCrossProviderDependencies_SameProvider_Empty(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("a", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("b", "aws-ecs")),
	)
	deps := BuildCrossProviderDependencies(plan)
	if len(deps) != 0 {
		t.Fatalf("same-provider stages must produce no cross-provider deps, got %d", len(deps))
	}
}

func TestBuildCrossProviderDependencies_DifferentProviders(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("db", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("api", "gcp-cloudrun")),
	)
	deps := BuildCrossProviderDependencies(plan)
	if len(deps) == 0 {
		t.Fatal("different-provider stages must produce cross-provider deps")
	}
	if deps[0].DependencyID == "" {
		t.Fatal("DependencyID must be non-empty")
	}
	if deps[0].WhyExists == "" {
		t.Fatal("WhyExists must be non-empty")
	}
}

func TestBuildCrossProviderDependencies_Nil(t *testing.T) {
	deps := BuildCrossProviderDependencies(nil)
	if len(deps) != 0 {
		t.Fatal("nil plan must produce no deps")
	}
}

// ─── snapshot ──────────────────────────────────────────────────────────────

func TestExportTopologySnapshot_NonNil(t *testing.T) {
	g := BuildTopologyGraph(makePlan(makeStage("s0", 0, nil, makeNode("a", "aws-ecs"))))
	snap := ExportTopologySnapshot(g, nil, nil, nil)
	if snap == nil {
		t.Fatal("ExportTopologySnapshot must return non-nil")
	}
}

func TestExportTopologySnapshot_HashIs64Chars(t *testing.T) {
	g := BuildTopologyGraph(nil)
	snap := ExportTopologySnapshot(g, nil, nil, nil)
	if len(snap.TopologyHash) != 64 {
		t.Fatalf("TopologyHash must be 64 hex chars, got %d", len(snap.TopologyHash))
	}
}

func TestVerifyTopologySnapshot_Valid(t *testing.T) {
	g := BuildTopologyGraph(makePlan(makeStage("s0", 0, nil, makeNode("a", "aws-ecs"))))
	snap := ExportTopologySnapshot(g, nil, nil, nil)
	ok, reason := VerifyTopologySnapshot(snap)
	if !ok {
		t.Fatalf("original snapshot must verify: %s", reason)
	}
}

func TestVerifyTopologySnapshot_TamperedExecutionID(t *testing.T) {
	g := BuildTopologyGraph(nil)
	snap := ExportTopologySnapshot(g, nil, nil, nil)
	snap.ExecutionID = "tampered"
	ok, _ := VerifyTopologySnapshot(snap)
	if ok {
		t.Fatal("tampered ExecutionID must fail verification")
	}
}

func TestVerifyTopologySnapshot_Nil(t *testing.T) {
	ok, reason := VerifyTopologySnapshot(nil)
	if ok || reason == "" {
		t.Fatal("nil snapshot must return false with non-empty reason")
	}
}

func TestExportTopologySnapshot_Deterministic(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, makeNode("a", "aws-ecs")),
		makeStage("s1", 1, []string{"s0"}, makeNode("b", "gcp-cloudrun")),
	)
	g := BuildTopologyGraph(plan)
	caps := BuildCapabilityMatrix(ProvidersFromGraph(g))
	domains := BuildFailureDomains(g)
	deps := BuildCrossProviderDependencies(plan)
	snap1 := ExportTopologySnapshot(g, caps, domains, deps)
	snap2 := ExportTopologySnapshot(g, caps, domains, deps)
	if snap1.TopologyHash != snap2.TopologyHash {
		t.Fatal("same inputs must produce same TopologyHash")
	}
}

func TestExplainTopology_LimitationsNonEmpty(t *testing.T) {
	g := BuildTopologyGraph(nil)
	exp := ExplainTopology(g, nil)
	if len(exp.Limitations) == 0 {
		t.Fatal("ExplainTopology must include limitations")
	}
}
