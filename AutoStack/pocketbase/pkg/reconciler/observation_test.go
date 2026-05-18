package reconciler

import (
	"testing"
	"time"
)

// Phase 3.9.7 — Provider observation pure-logic tests.
//
// SCOPE STATEMENT (honest):
// These tests verify the structural correctness of observation types,
// the pure-function confidence engine, inline contradiction detection,
// staleness classification, and the registry completeness invariants.
//
// DB-backed tests (RecordProviderObservation writes, ListProviderObservations
// queries, drift persistence) require a PocketBase test fixture that this
// codebase does not have yet. The DB path is validated by observing
// [OBS_RECORDED] and [DRIFT_OPENED] log lines under real runtime.
//
// What we CAN prove without a DB:
//   - all string constants are stable (schema correctness)
//   - registry slices are non-empty and internally consistent
//   - DetectObservationContradiction covers the required cases
//   - ClassifyProviderConfidence is a total function over all trust levels
//   - IsObservationStale classifies correctly against the staleness window
//   - DriftRecord and ProviderObservationRecord structs are well-formed

// ─────────────────────────────────────────────────────────────────────────
// STABLE STRING TESTS
// ─────────────────────────────────────────────────────────────────────────

func TestObservationSourceStableStrings(t *testing.T) {
	expected := map[ObservationSource]string{
		ObservationSourcePoll:               "poll",
		ObservationSourceDeployResult:       "deploy-result",
		ObservationSourceRollbackResult:     "rollback-result",
		ObservationSourceReconciliationRead: "reconciliation-read",
		ObservationSourceStartupRehydrate:   "startup-rehydrate",
	}
	for src, want := range expected {
		if string(src) != want {
			t.Errorf("ObservationSource %q changed to %q — BREAKING SCHEMA CHANGE", want, src)
		}
	}
}

func TestObservationTrustStableStrings(t *testing.T) {
	expected := map[ObservationTrust]string{
		ObservationTrustAuthoritative: "authoritative",
		ObservationTrustInferred:      "inferred",
		ObservationTrustStale:         "stale",
		ObservationTrustAmbiguous:     "ambiguous",
		ObservationTrustContradicted:  "contradicted",
	}
	for trust, want := range expected {
		if string(trust) != want {
			t.Errorf("ObservationTrust %q changed to %q — BREAKING SCHEMA CHANGE", want, trust)
		}
	}
}

func TestProviderConfidenceClassStableStrings(t *testing.T) {
	expected := map[ProviderConfidenceClass]string{
		ProviderConfidenceAuthoritative: "authoritative_provider",
		ProviderConfidenceInferred:      "inferred_provider",
		ProviderConfidenceStaleCache:    "stale_cache",
		ProviderConfidenceAmbiguous:     "ambiguous_result",
		ProviderConfidenceContradicted:  "contradiction_detected",
		ProviderConfidenceAssumption:    "runtime_assumption",
	}
	for cls, want := range expected {
		if string(cls) != want {
			t.Errorf("ProviderConfidenceClass %q changed to %q — BREAKING SCHEMA CHANGE", want, cls)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// REGISTRY COMPLETENESS
// ─────────────────────────────────────────────────────────────────────────

func TestAllObservationSourcesRegistryComplete(t *testing.T) {
	if len(AllObservationSources) == 0 {
		t.Fatal("AllObservationSources must contain at least one source")
	}
	if len(AllObservationSources) < 5 {
		t.Errorf("AllObservationSources has %d entries; initial Phase 3.9.7 set was 5", len(AllObservationSources))
	}
	seen := map[ObservationSource]bool{}
	for _, s := range AllObservationSources {
		if s == "" {
			t.Error("AllObservationSources contains empty string")
		}
		if seen[s] {
			t.Errorf("AllObservationSources contains duplicate %q", s)
		}
		seen[s] = true
	}
}

func TestAllObservationTrustLevelsRegistryComplete(t *testing.T) {
	if len(AllObservationTrustLevels) == 0 {
		t.Fatal("AllObservationTrustLevels must contain at least one level")
	}
	if len(AllObservationTrustLevels) < 5 {
		t.Errorf("AllObservationTrustLevels has %d entries; initial Phase 3.9.7 set was 5", len(AllObservationTrustLevels))
	}
	seen := map[ObservationTrust]bool{}
	for _, t2 := range AllObservationTrustLevels {
		if t2 == "" {
			t.Error("AllObservationTrustLevels contains empty string")
		}
		if seen[t2] {
			t.Errorf("AllObservationTrustLevels contains duplicate %q", t2)
		}
		seen[t2] = true
	}
}

func TestAllProviderConfidenceClassesRegistryComplete(t *testing.T) {
	if len(AllProviderConfidenceClasses) < 6 {
		t.Errorf("AllProviderConfidenceClasses has %d entries; initial Phase 3.9.7 set was 6", len(AllProviderConfidenceClasses))
	}
	seen := map[ProviderConfidenceClass]bool{}
	for _, c := range AllProviderConfidenceClasses {
		if c == "" {
			t.Error("AllProviderConfidenceClasses contains empty string")
		}
		if seen[c] {
			t.Errorf("AllProviderConfidenceClasses contains duplicate %q", c)
		}
		seen[c] = true
	}
}

// ─────────────────────────────────────────────────────────────────────────
// ClassifyProviderConfidence — pure function
// ─────────────────────────────────────────────────────────────────────────

func TestClassifyProviderConfidenceTotalFunction(t *testing.T) {
	// Every ObservationTrust must produce a valid ProviderConfidenceClass.
	for _, trust := range AllObservationTrustLevels {
		got := ClassifyProviderConfidence(trust)
		if got == "" {
			t.Errorf("ClassifyProviderConfidence(%q) returned empty string", trust)
		}
		// Verify it's in the known set.
		found := false
		for _, cls := range AllProviderConfidenceClasses {
			if cls == got {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ClassifyProviderConfidence(%q) = %q, not in AllProviderConfidenceClasses", trust, got)
		}
	}
}

func TestClassifyProviderConfidenceMapping(t *testing.T) {
	cases := []struct {
		trust ObservationTrust
		want  ProviderConfidenceClass
	}{
		{ObservationTrustAuthoritative, ProviderConfidenceAuthoritative},
		{ObservationTrustInferred, ProviderConfidenceInferred},
		{ObservationTrustStale, ProviderConfidenceStaleCache},
		{ObservationTrustAmbiguous, ProviderConfidenceAmbiguous},
		{ObservationTrustContradicted, ProviderConfidenceContradicted},
	}
	for _, tc := range cases {
		got := ClassifyProviderConfidence(tc.trust)
		if got != tc.want {
			t.Errorf("ClassifyProviderConfidence(%q) = %q, want %q", tc.trust, got, tc.want)
		}
	}
}

func TestClassifyProviderConfidenceUnknownTrustFallsBack(t *testing.T) {
	// An unknown trust value should return the safe fallback.
	got := ClassifyProviderConfidence("some_unknown_trust_level")
	if got != ProviderConfidenceAssumption {
		t.Errorf("unknown trust returned %q, want %q", got, ProviderConfidenceAssumption)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// IsObservationStale — pure function
// ─────────────────────────────────────────────────────────────────────────

func TestIsObservationStaleNotStale(t *testing.T) {
	// An observation 30 seconds old should not be stale.
	recent := time.Now().Add(-30 * time.Second)
	if IsObservationStale(recent) {
		t.Error("IsObservationStale returned true for 30-second-old observation; want false")
	}
}

func TestIsObservationStaleJustUnderWindow(t *testing.T) {
	// 1 second under the staleness window should be non-stale.
	justUnder := time.Now().Add(-(observationStalenessWindow - time.Second))
	if IsObservationStale(justUnder) {
		t.Error("IsObservationStale returned true for observation just under window; want false")
	}
}

func TestIsObservationStaleExceedsWindow(t *testing.T) {
	// An observation 1 second over the staleness window should be stale.
	stale := time.Now().Add(-(observationStalenessWindow + time.Second))
	if !IsObservationStale(stale) {
		t.Error("IsObservationStale returned false for observation past window; want true")
	}
}

func TestIsObservationStaleOldObservation(t *testing.T) {
	// A 24-hour-old observation must be stale.
	old := time.Now().Add(-24 * time.Hour)
	if !IsObservationStale(old) {
		t.Error("IsObservationStale returned false for 24-hour-old observation; want true")
	}
}

func TestObservationStalenessWindowIsReasonable(t *testing.T) {
	// The staleness window must be at least 60 seconds (2× the minimum poll
	// interval) and at most 10 minutes (otherwise it would not catch real
	// polling failures).
	if observationStalenessWindow < 60*time.Second {
		t.Errorf("observationStalenessWindow=%v is too short; must be ≥60s", observationStalenessWindow)
	}
	if observationStalenessWindow > 10*time.Minute {
		t.Errorf("observationStalenessWindow=%v is too long; must be ≤10m", observationStalenessWindow)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// DetectObservationContradiction — pure function
// ─────────────────────────────────────────────────────────────────────────

func TestDetectObservationContradictionMissingVsRunning(t *testing.T) {
	ok, reason := DetectObservationContradiction("missing", "running")
	if !ok {
		t.Error("missing/running should be a contradiction")
	}
	if reason == "" {
		t.Error("contradiction reason must be non-empty")
	}
}

func TestDetectObservationContradictionDeletedVsActive(t *testing.T) {
	ok, reason := DetectObservationContradiction("deleted", "active")
	if !ok {
		t.Error("deleted/active should be a contradiction")
	}
	if reason == "" {
		t.Error("contradiction reason must be non-empty")
	}
}

func TestDetectObservationContradictionRunningVsRolledBack(t *testing.T) {
	ok, reason := DetectObservationContradiction("running", "rolled_back")
	if !ok {
		t.Error("running/rolled_back should be a contradiction")
	}
	if reason == "" {
		t.Error("contradiction reason must be non-empty")
	}
}

func TestDetectObservationContradictionActiveVsRollbackComplete(t *testing.T) {
	ok, _ := DetectObservationContradiction("active", "rollback_complete")
	if !ok {
		t.Error("active/rollback_complete should be a contradiction")
	}
}

func TestDetectObservationContradictionNotFoundVsDeployed(t *testing.T) {
	ok, _ := DetectObservationContradiction("not_found", "deployed")
	if !ok {
		t.Error("not_found/deployed should be a contradiction")
	}
}

func TestDetectObservationContradictionNoContradictionSameState(t *testing.T) {
	// Same state on both sides is never a contradiction.
	ok, _ := DetectObservationContradiction("running", "running")
	if ok {
		t.Error("running/running should NOT be a contradiction")
	}
}

func TestDetectObservationContradictionNoContradictionExpectedError(t *testing.T) {
	// Runtime expects error, provider says error — not a contradiction
	// (runtime already knows about the problem).
	ok, _ := DetectObservationContradiction("error", "error")
	if ok {
		t.Error("error/error should NOT be a contradiction")
	}
}

func TestDetectObservationContradictionEmptyInputs(t *testing.T) {
	// Empty inputs must not panic and must return false.
	ok, _ := DetectObservationContradiction("", "running")
	if ok {
		t.Error("empty providerState should not be a contradiction")
	}
	ok, _ = DetectObservationContradiction("running", "")
	if ok {
		t.Error("empty runtimeExpectedState should not be a contradiction")
	}
	ok, _ = DetectObservationContradiction("", "")
	if ok {
		t.Error("both empty should not be a contradiction")
	}
}

func TestDetectObservationContradictionUnknownStates(t *testing.T) {
	// Unknown state combinations are NOT contradictions (fail-open).
	// Adding new states should not suddenly make existing pairings contradictions.
	ok, _ := DetectObservationContradiction("provisioning", "deploying")
	if ok {
		t.Error("unknown states provisioning/deploying should not be a contradiction (fail-open)")
	}
}

func TestContradictionStateMapConsistency(t *testing.T) {
	// Verify no entry maps a provider state to itself as a contradiction.
	for providerState, expectedStates := range contradictionStateMap {
		for _, expected := range expectedStates {
			if providerState == expected {
				t.Errorf("contradictionStateMap[%q] contains itself — a state cannot contradict itself", providerState)
			}
		}
		if len(expectedStates) == 0 {
			t.Errorf("contradictionStateMap[%q] has empty expected-states list — remove the key instead", providerState)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// STRUCT FIELD VALIDATION
// ─────────────────────────────────────────────────────────────────────────

func TestProviderObservationInputZeroValue(t *testing.T) {
	// A zero-value input must not panic when inspected.
	var obs ProviderObservationInput
	if obs.TargetID != "" {
		t.Error("zero ProviderObservationInput should have empty TargetID")
	}
	if obs.ContradictionDetected {
		t.Error("zero ProviderObservationInput should not have ContradictionDetected=true")
	}
	if !obs.ObservedAt.IsZero() {
		t.Error("zero ProviderObservationInput should have zero ObservedAt")
	}
}

func TestDriftRecordZeroValue(t *testing.T) {
	// A zero-value DriftRecord must not panic.
	var dr DriftRecord
	if dr.DriftCount != 0 {
		t.Error("zero DriftRecord should have DriftCount=0")
	}
	if dr.ResolvedAt != "" {
		t.Error("zero DriftRecord should have empty ResolvedAt (active drift)")
	}
}

func TestProviderObservationRecordZeroValue(t *testing.T) {
	var por ProviderObservationRecord
	if por.ContradictionDetected {
		t.Error("zero ProviderObservationRecord should not be contradicted")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// CONTRADICTION DECISION TYPE — STABLE STRINGS
// ─────────────────────────────────────────────────────────────────────────

func TestPhase397DecisionTypeStableStrings(t *testing.T) {
	expected := map[DecisionType]string{
		DecisionProviderContradiction: "provider_contradiction",
		DecisionProviderUnknowable:    "provider_unknowable",
		DecisionDriftOpened:           "drift_opened",
		DecisionDriftResolved:         "drift_resolved",
	}
	for dt, want := range expected {
		if string(dt) != want {
			t.Errorf("Phase 3.9.7 DecisionType %q changed to %q — BREAKING SCHEMA CHANGE", want, dt)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// INCIDENT SOURCE — STABLE STRINGS
// ─────────────────────────────────────────────────────────────────────────

func TestPhase397IncidentSourceStableStrings(t *testing.T) {
	expected := map[IncidentSource]string{
		IncidentSourceProviderContradiction: "provider-contradiction",
		IncidentSourceProviderUnknowable:    "provider-unknowable",
		IncidentSourceDriftPersistent:       "drift-persistent",
	}
	for src, want := range expected {
		if string(src) != want {
			t.Errorf("Phase 3.9.7 IncidentSource %q changed to %q — BREAKING SCHEMA CHANGE", want, src)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// CONTRADICTION REGISTRY — PHASE 3.9.7 ADDITIONS
// ─────────────────────────────────────────────────────────────────────────

func TestPhase397ContradictionChecksPresent(t *testing.T) {
	required := []string{
		"ProviderDriftExceedingEscalationThreshold",
		"RepeatedAmbiguousObservationsWithoutResolution",
	}
	seen := map[string]bool{}
	for _, c := range AllContradictions {
		seen[c.Name] = true
	}
	for _, name := range required {
		if !seen[name] {
			t.Errorf("Phase 3.9.7 contradiction check %q is missing from AllContradictions", name)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// METRICS HELPERS — ZERO-VALUE SAFETY
// ─────────────────────────────────────────────────────────────────────────

func TestMetricsProviderObservationIncSafe(t *testing.T) {
	var m Metrics
	// Should not panic on nil map.
	m.IncProviderObservation("authoritative")
	m.IncProviderObservation("contradicted")
	snap := m.Snapshot()
	if snap.ProviderObsByTrust["authoritative"] != 1 {
		t.Errorf("ProviderObsByTrust[authoritative] = %d, want 1", snap.ProviderObsByTrust["authoritative"])
	}
	if snap.ProviderObsByTrust["contradicted"] != 1 {
		t.Errorf("ProviderObsByTrust[contradicted] = %d, want 1", snap.ProviderObsByTrust["contradicted"])
	}
}

func TestMetricsProviderContradictionIncSafe(t *testing.T) {
	var m Metrics
	m.IncProviderContradiction()
	m.IncProviderContradiction()
	snap := m.Snapshot()
	if snap.ProviderContradictions != 2 {
		t.Errorf("ProviderContradictions = %d, want 2", snap.ProviderContradictions)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// DRIFT RESOLUTION VALIDATION
// ─────────────────────────────────────────────────────────────────────────

func TestResolveProviderDriftRequiresReason(t *testing.T) {
	// We cannot call ResolveProviderDrift with an empty reason.
	// Verify the validation function logic without a DB by constructing
	// a minimal Reconciler with nil app (should return error before DB call).
	r := &Reconciler{}
	err := r.ResolveProviderDrift("some-drift-id", "")
	if err == nil {
		t.Error("ResolveProviderDrift with empty reason should return error")
	}
}

func TestResolveProviderDriftRequiresDriftID(t *testing.T) {
	r := &Reconciler{}
	err := r.ResolveProviderDrift("", "some reason")
	if err == nil {
		t.Error("ResolveProviderDrift with empty driftID should return error")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// POLICY VERSION BUMP
// ─────────────────────────────────────────────────────────────────────────

func TestPolicyVersionBumpedForPhase397(t *testing.T) {
	// PolicyVersion must be at least 3.9.7.x.
	// We don't parse it as semver; just verify it contains "3.9.7".
	if len(PolicyVersion) < 5 {
		t.Fatalf("PolicyVersion=%q is too short to contain a valid version", PolicyVersion)
	}
	if PolicyVersion < "3.9.7" {
		t.Errorf("PolicyVersion=%q should be ≥3.9.7 after Phase 3.9.7 changes", PolicyVersion)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// NARRATIVE SOURCE PRIORITY
// ─────────────────────────────────────────────────────────────────────────

func TestNarrativeSourcePriorityIncludesObservation(t *testing.T) {
	if _, ok := sourcePriority["observation"]; !ok {
		t.Error("sourcePriority must include 'observation' (Phase 3.9.7 provider truth entries)")
	}
}

func TestNarrativeObservationPriorityAfterIncident(t *testing.T) {
	incidentPriority, incidentOK := sourcePriority["incident"]
	obsPriority, obsOK := sourcePriority["observation"]
	if !incidentOK || !obsOK {
		t.Skip("incident or observation not in sourcePriority")
	}
	// Observations come after incidents in the narrative (incidents describe
	// the operational impact; observations describe the provider evidence).
	if obsPriority <= incidentPriority {
		t.Errorf("observation priority (%d) should be > incident priority (%d)", obsPriority, incidentPriority)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// FORENSIC SNAPSHOT — FIELD PRESENCE
// ─────────────────────────────────────────────────────────────────────────

func TestForensicSnapshotHasProviderFields(t *testing.T) {
	// Verify the struct has the Phase 3.9.7 fields via a zero-value check.
	var snap ForensicSnapshot
	if snap.ProviderObservations != nil {
		t.Error("zero ForensicSnapshot should have nil ProviderObservations")
	}
	if snap.ProviderDrifts != nil {
		t.Error("zero ForensicSnapshot should have nil ProviderDrifts")
	}
}
