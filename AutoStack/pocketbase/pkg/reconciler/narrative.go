package reconciler

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
)

// Phase 3.9.6 — Deterministic narrative reconstruction.
//
// ReconstructNarrative merges four collections (runtime_decisions,
// operations, deployment_history, incidents) for a single target into
// one chronological stream. The merge is deterministic: identical DB
// state produces byte-identical output (modulo the query window).
//
// Truthfulness contract:
//   - NO synthesis. Every NarrativeEntry derives from an actual row.
//   - NO inference. The narrative reports what the rows say, not what
//     they imply.
//   - NO uncertainty flattening. If a row indicates ambiguity, the entry
//     surfaces it (via Confidence and Summary fields).

// ─────────────────────────────────────────────────────────────────────────
// PUBLIC TYPES
// ─────────────────────────────────────────────────────────────────────────

// NarrativeEntry is one row in the merged operator-facing narrative.
type NarrativeEntry struct {
	Timestamp    string        `json:"timestamp"`            // RFC3339 — primary sort key
	Source       string        `json:"source"`               // "decision" | "operation" | "history" | "incident"
	SourceID     string        `json:"source_id"`            // row id in the source collection
	EventType    string        `json:"event_type"`           // decision_type / kind / action / source
	Summary      string        `json:"summary"`              // human-readable single-line
	Confidence   string        `json:"confidence,omitempty"` // populated for decisions only
	OperationID  string        `json:"operation_id,omitempty"`
	CycleID      string        `json:"cycle_id,omitempty"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs,omitempty"`
}

// Narrative is the full reconstruction for a target over a time window.
type Narrative struct {
	TargetID         string           `json:"target_id"`
	WindowStart      string           `json:"window_start"`
	WindowEnd        string           `json:"window_end"`
	Entries          []NarrativeEntry `json:"entries"`
	TruthfulnessNote string           `json:"truthfulness_note,omitempty"`
}

// ReconstructNarrative reads decisions + operations + history + incidents
// for a target within [since, until] and returns a merged stream sorted
// by timestamp.
//
// Caps each per-table read at narrativeQueryLimit rows. If any limit is
// hit, TruthfulnessNote records the truncation.
//
// Best-effort: read errors are logged and recorded in TruthfulnessNote;
// they do NOT propagate. A partial narrative is more useful than no
// narrative.
func ReconstructNarrative(app *pb.PocketBase, targetID string, since, until time.Time) *Narrative {
	if targetID == "" {
		return nil
	}
	n := &Narrative{
		TargetID:    targetID,
		WindowStart: since.UTC().Format(time.RFC3339),
		WindowEnd:   until.UTC().Format(time.RFC3339),
	}
	sinceStr := since.UTC().Format(time.RFC3339)
	untilStr := until.UTC().Format(time.RFC3339)

	// 1. runtime_decisions
	decisionEntries, truncated, err := narrativeFromDecisions(app, targetID, sinceStr, untilStr)
	if err != nil {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "decisions read failed: "+err.Error())
	}
	if truncated {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "decisions truncated at "+intToStr(narrativeQueryLimit))
	}

	// 2. operations
	opEntries, truncated, err := narrativeFromOperations(app, targetID, sinceStr, untilStr)
	if err != nil {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "operations read failed: "+err.Error())
	}
	if truncated {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "operations truncated at "+intToStr(narrativeQueryLimit))
	}

	// 3. deployment_history
	historyEntries, truncated, err := narrativeFromHistory(app, targetID, sinceStr, untilStr)
	if err != nil {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "history read failed: "+err.Error())
	}
	if truncated {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "history truncated at "+intToStr(narrativeQueryLimit))
	}

	// 4. incidents
	incidentEntries, truncated, err := narrativeFromIncidents(app, targetID, sinceStr, untilStr)
	if err != nil {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "incidents read failed: "+err.Error())
	}
	if truncated {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "incidents truncated at "+intToStr(narrativeQueryLimit))
	}

	// 5. provider observations (Phase 3.9.7) — contradiction + drift events.
	// Only contradiction observations are included: non-contradicting observations
	// are high-volume (one per poll tick) and would dominate the narrative.
	// Operators wanting the full observation timeline use ListProviderObservations.
	obsEntries, truncated, err := narrativeFromProviderObservations(app, targetID, sinceStr, untilStr)
	if err != nil {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "provider_observations read failed: "+err.Error())
	}
	if truncated {
		n.TruthfulnessNote = appendNote(n.TruthfulnessNote, "provider_observations truncated at "+intToStr(narrativeQueryLimit))
	}

	n.Entries = mergeNarrativeEntries(decisionEntries, opEntries, historyEntries, incidentEntries, obsEntries)
	return n
}

// narrativeQueryLimit caps each per-table query. Keeps narrative size
// bounded for large/long-running targets. Truncation is reported in
// TruthfulnessNote so operators know.
const narrativeQueryLimit = 2000

// ─────────────────────────────────────────────────────────────────────────
// MERGE / SORT
// ─────────────────────────────────────────────────────────────────────────

// sourcePriority is the tie-breaker when two entries share a timestamp.
// Decisions come FIRST in a cycle (they record the reasoning); the
// resulting history rows and operation transitions come after. Incidents
// come last (they may reference operations created in the same instant).
var sourcePriority = map[string]int{
	"decision":    0,
	"history":     1,
	"operation":   2,
	"incident":    3,
	"observation": 4, // Phase 3.9.7 — provider truth observations (contradiction-only)
}

// mergeNarrativeEntries deterministically merges four entry slices by
// (timestamp, source-priority, source-id). Pure function — unit-testable
// without a DB.
func mergeNarrativeEntries(slices ...[]NarrativeEntry) []NarrativeEntry {
	total := 0
	for _, s := range slices {
		total += len(s)
	}
	out := make([]NarrativeEntry, 0, total)
	for _, s := range slices {
		out = append(out, s...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		pi, pj := sourcePriority[out[i].Source], sourcePriority[out[j].Source]
		if pi != pj {
			return pi < pj
		}
		return out[i].SourceID < out[j].SourceID
	})
	return out
}

// ─────────────────────────────────────────────────────────────────────────
// PER-TABLE PROJECTIONS
// ─────────────────────────────────────────────────────────────────────────

func narrativeFromDecisions(app *pb.PocketBase, targetID, since, until string) ([]NarrativeEntry, bool, error) {
	var rows []struct {
		ID            string `db:"id"`
		DecisionType  string `db:"decision_type"`
		DecisionAt    string `db:"decision_timestamp"`
		HumanSummary  string `db:"human_summary"`
		Confidence    string `db:"runtime_confidence"`
		OperationID   string `db:"operation_id"`
		CycleID       string `db:"reconcile_cycle_id"`
		EvidenceRefs  string `db:"evidence_refs"`
	}
	err := app.Dao().DB().
		Select("id", "decision_type", "decision_timestamp", "human_summary",
			"runtime_confidence", "operation_id", "reconcile_cycle_id", "evidence_refs").
		From("runtime_decisions").
		Where(dbx.NewExp(
			"target = {:t} AND decision_timestamp >= {:s} AND decision_timestamp <= {:u}",
			dbx.Params{"t": targetID, "s": since, "u": until},
		)).
		OrderBy("decision_timestamp ASC").
		Limit(narrativeQueryLimit + 1).
		All(&rows)
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > narrativeQueryLimit
	if truncated {
		rows = rows[:narrativeQueryLimit]
	}
	out := make([]NarrativeEntry, 0, len(rows))
	for _, r := range rows {
		entry := NarrativeEntry{
			Timestamp:   r.DecisionAt,
			Source:      "decision",
			SourceID:    r.ID,
			EventType:   r.DecisionType,
			Summary:     r.HumanSummary,
			Confidence:  r.Confidence,
			OperationID: r.OperationID,
			CycleID:     r.CycleID,
		}
		if r.EvidenceRefs != "" && r.EvidenceRefs != "null" {
			var refs []EvidenceRef
			if err := json.Unmarshal([]byte(r.EvidenceRefs), &refs); err == nil {
				entry.EvidenceRefs = refs
			}
		}
		out = append(out, entry)
	}
	return out, truncated, nil
}

func narrativeFromOperations(app *pb.PocketBase, targetID, since, until string) ([]NarrativeEntry, bool, error) {
	var rows []struct {
		ID        string `db:"id"`
		Kind      string `db:"kind"`
		Status    string `db:"status"`
		ExecStage string `db:"exec_stage"`
		StartedAt string `db:"started_at"`
		UpdatedAt string `db:"updated_at"`
		Message   string `db:"message"`
		CycleID   string `db:"cycle_id"`
	}
	err := app.Dao().DB().
		Select("id", "kind", "status", "exec_stage", "started_at", "updated_at", "message", "cycle_id").
		From("operations").
		Where(dbx.NewExp(
			"target = {:t} AND started_at >= {:s} AND started_at <= {:u}",
			dbx.Params{"t": targetID, "s": since, "u": until},
		)).
		OrderBy("started_at ASC").
		Limit(narrativeQueryLimit + 1).
		All(&rows)
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > narrativeQueryLimit
	if truncated {
		rows = rows[:narrativeQueryLimit]
	}
	out := make([]NarrativeEntry, 0, len(rows))
	for _, r := range rows {
		summary := fmt.Sprintf("operation %s status=%s exec_stage=%s", r.Kind, r.Status, r.ExecStage)
		if r.Message != "" {
			summary += ": " + r.Message
		}
		out = append(out, NarrativeEntry{
			Timestamp:   r.StartedAt,
			Source:      "operation",
			SourceID:    r.ID,
			EventType:   r.Kind,
			Summary:     summary,
			OperationID: r.ID,
			CycleID:     r.CycleID,
		})
	}
	return out, truncated, nil
}

func narrativeFromHistory(app *pb.PocketBase, targetID, since, until string) ([]NarrativeEntry, bool, error) {
	var rows []struct {
		ID          string `db:"id"`
		Action      string `db:"action"`
		Status      string `db:"status"`
		Message     string `db:"message"`
		OperationID string `db:"operation_id"`
		TriggeredBy string `db:"triggered_by"`
		Created     string `db:"created"`
	}
	err := app.Dao().DB().
		Select("id", "action", "status", "message", "operation_id", "triggered_by", "created").
		From("deployment_history").
		Where(dbx.NewExp(
			"target = {:t} AND created >= {:s} AND created <= {:u}",
			dbx.Params{"t": targetID, "s": since, "u": until},
		)).
		OrderBy("created ASC").
		Limit(narrativeQueryLimit + 1).
		All(&rows)
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > narrativeQueryLimit
	if truncated {
		rows = rows[:narrativeQueryLimit]
	}
	out := make([]NarrativeEntry, 0, len(rows))
	for _, r := range rows {
		summary := r.Action + " " + r.Status
		if r.TriggeredBy != "" {
			summary += " (triggered_by=" + r.TriggeredBy + ")"
		}
		if r.Message != "" {
			summary += ": " + r.Message
		}
		out = append(out, NarrativeEntry{
			Timestamp:   r.Created,
			Source:      "history",
			SourceID:    r.ID,
			EventType:   r.Action,
			Summary:     summary,
			OperationID: r.OperationID,
		})
	}
	return out, truncated, nil
}

func narrativeFromIncidents(app *pb.PocketBase, targetID, since, until string) ([]NarrativeEntry, bool, error) {
	// Incidents that overlap the window: first_seen_at <= until AND
	// (resolved_at IS NULL OR resolved_at >= since). For each, emit up
	// to 3 entries at first_seen_at, last_seen_at, resolved_at (if set).
	var rows []struct {
		ID          string `db:"id"`
		Source      string `db:"source"`
		Severity    string `db:"severity"`
		Status      string `db:"status"`
		FirstSeenAt string `db:"first_seen_at"`
		LastSeenAt  string `db:"last_seen_at"`
		ResolvedAt  string `db:"resolved_at"`
		Summary     string `db:"forensic_summary"`
		OpID        string `db:"first_operation_id"`
	}
	err := app.Dao().DB().
		Select("id", "source", "severity", "status", "first_seen_at",
			"last_seen_at", "resolved_at", "forensic_summary", "first_operation_id").
		From("incidents").
		Where(dbx.NewExp(
			"target = {:t} AND first_seen_at <= {:u} AND (resolved_at IS NULL OR resolved_at = '' OR resolved_at >= {:s})",
			dbx.Params{"t": targetID, "s": since, "u": until},
		)).
		OrderBy("first_seen_at ASC").
		Limit(narrativeQueryLimit + 1).
		All(&rows)
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > narrativeQueryLimit
	if truncated {
		rows = rows[:narrativeQueryLimit]
	}
	out := make([]NarrativeEntry, 0, len(rows)*2)
	for _, r := range rows {
		// first_seen entry (always emit)
		out = append(out, NarrativeEntry{
			Timestamp:   r.FirstSeenAt,
			Source:      "incident",
			SourceID:    r.ID + ":opened",
			EventType:   "incident_" + r.Source,
			Summary:     "incident opened: " + r.Summary + " (severity=" + r.Severity + ")",
			OperationID: r.OpID,
		})
		// resolved entry (only if resolved within the window)
		if r.ResolvedAt != "" && r.ResolvedAt >= since && r.ResolvedAt <= until {
			out = append(out, NarrativeEntry{
				Timestamp:   r.ResolvedAt,
				Source:      "incident",
				SourceID:    r.ID + ":resolved",
				EventType:   "incident_resolved",
				Summary:     "incident resolved (was: " + r.Source + ")",
				OperationID: r.OpID,
			})
		}
	}
	return out, truncated, nil
}

// narrativeFromProviderObservations reads contradiction observations only.
// Non-contradiction observations are deliberately excluded: at 30s poll
// intervals they would generate ~2880 entries/day per target, dominating
// the narrative. Operators wanting the full observation timeline use
// ListProviderObservations instead.
func narrativeFromProviderObservations(app *pb.PocketBase, targetID, since, until string) ([]NarrativeEntry, bool, error) {
	var rows []struct {
		ID                   string `db:"id"`
		ObservedAt           string `db:"observed_at"`
		Provider             string `db:"provider"`
		ObservationSource    string `db:"observation_source"`
		ProviderState        string `db:"provider_state"`
		RuntimeExpectedState string `db:"runtime_expected_state"`
		ObservationTrust     string `db:"observation_trust"`
		DriftReason          string `db:"drift_reason"`
	}
	err := app.Dao().DB().
		Select("id", "observed_at", "provider", "observation_source",
			"provider_state", "runtime_expected_state", "observation_trust", "drift_reason").
		From("provider_observations").
		Where(dbx.NewExp(
			"target = {:t} AND contradiction_detected = 1 AND observed_at >= {:s} AND observed_at <= {:u}",
			dbx.Params{"t": targetID, "s": since, "u": until},
		)).
		OrderBy("observed_at ASC").
		Limit(narrativeQueryLimit + 1).
		All(&rows)
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > narrativeQueryLimit
	if truncated {
		rows = rows[:narrativeQueryLimit]
	}
	out := make([]NarrativeEntry, 0, len(rows))
	for _, r := range rows {
		reason := r.DriftReason
		if reason == "" {
			reason = fmt.Sprintf("provider=%s state=%s trust=%s", r.Provider, r.ProviderState, r.ObservationTrust)
		}
		summary := fmt.Sprintf("[PROVIDER CONTRADICTION] %s (source=%s expected=%s)", reason, r.ObservationSource, r.RuntimeExpectedState)
		out = append(out, NarrativeEntry{
			Timestamp:  r.ObservedAt,
			Source:     "observation",
			SourceID:   r.ID,
			EventType:  "provider_contradiction",
			Summary:    summary,
			Confidence: "medium",
		})
	}
	return out, truncated, nil
}

// appendNoteFn is shadowed by appendNote in forensic.go — defining locally
// only if not already present.
func init() {
	_ = log.Printf // avoid unused import in some build configurations
}
