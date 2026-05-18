package reconciler

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
)

// Phase 3.5 — IncidentTimeline Explanation Model.
//
// An "incident" is a reconstructed operational story: the sequence of
// operations, history rows, and lifecycle transitions for a single target
// over a time window. Operators use this to answer:
//
//   - what happened
//   - why it happened
//   - what state was authoritative
//   - whether provider truth was trusted
//   - whether ambiguity existed
//   - whether escalation occurred
//   - whether drift was detected
//
// The model is READ-ONLY: it reconstructs from existing operations and
// deployment_history rows. No new persisted state. This keeps the schema
// minimal while delivering operator-debug surfaces.
//
// Storage: nothing new. The data lives in operations.* and
// deployment_history.* — this file just exposes a structured reading.

// IncidentTimeline is a structured narrative for a single target over a time window.
type IncidentTimeline struct {
	TargetID      string             `json:"target_id"`
	RolloutID     string             `json:"rollout_id"`
	Provider      string             `json:"provider"`
	WindowStart   time.Time          `json:"window_start"`
	WindowEnd     time.Time          `json:"window_end"`
	CurrentStatus string             `json:"current_status"`
	DriftState    string             `json:"drift_state"`
	Ambiguous     bool               `json:"ambiguous"`
	Events        []IncidentTimelineEvent    `json:"events"`
	Operations    []IncidentTimelineOp       `json:"operations"`
	Summary       string             `json:"summary"`
}

// IncidentTimelineEvent is a single deployment_history entry rendered for an incident view.
type IncidentTimelineEvent struct {
	At          time.Time `json:"at"`
	Action      string    `json:"action"`        // created, updated, deleted, rollback, ambiguous
	Status      string    `json:"status"`        // in_progress, success, failed
	OperationID string    `json:"operation_id"`  // links to IncidentTimelineOp
	TriggeredBy string    `json:"triggered_by"`  // causal context: operator-rollback, stale-spec, ambiguity-expiry, etc.
	Message     string    `json:"message"`
}

// IncidentTimelineOp is a single operations row rendered for an incident view.
type IncidentTimelineOp struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`         // deploy, destroy, rollback
	Status      string    `json:"status"`       // in_progress, succeeded, failed, cancelled
	ExecStage   string    `json:"exec_stage"`   // queued, dispatching, provisioning, ..., ready, failed, ambiguous
	OwnedByPod  string    `json:"owned_by_pod"` // Phase 3.8 — pod identity of the reconciler that ran this op
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Message     string    `json:"message"`
	HasCheckpoint bool    `json:"has_checkpoint"`
	HasDeployedSpec bool  `json:"has_deployed_spec"` // Phase 3.5
}

// ReconstructIncidentTimeline reads operations + history for the target within the
// window and returns a structured IncidentTimeline. Returns nil on DB errors with
// the error logged — best-effort, never throws.
//
// The Summary field is a one-paragraph operator-readable rendering of the
// timeline highlights: status changes, escalations, ambiguity events,
// drift transitions.
func ReconstructIncidentTimeline(app *pb.PocketBase, targetID string, windowStart, windowEnd time.Time) *IncidentTimeline {
	if targetID == "" {
		return nil
	}

	// Read target current state.
	targetRec, err := app.Dao().FindRecordById("deployment_targets", targetID)
	if err != nil {
		log.Printf("[INCIDENT_READ_ERR] target=%s find: %v", targetID, err)
		return nil
	}

	incident := &IncidentTimeline{
		TargetID:      targetID,
		RolloutID:     targetRec.GetString("rollout"),
		Provider:      targetRec.GetString("provider"),
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		CurrentStatus: targetRec.GetString("status"),
		DriftState:    targetRec.GetString("drift_state"),
		Ambiguous:     targetRec.GetBool("lifecycle_ambiguous"),
	}

	// Read operations within window.
	var opRows []struct {
		ID            string `db:"id"`
		Kind          string `db:"kind"`
		Status        string `db:"status"`
		ExecStage     string `db:"exec_stage"`
		OwnedByPod    string `db:"owned_by_pod"`
		StartedAt     string `db:"started_at"`
		UpdatedAt     string `db:"updated_at"`
		Message       string `db:"message"`
		CheckpointData string `db:"checkpoint_data"`
		DeployedSpec  string `db:"deployed_spec"`
	}
	startStr := windowStart.UTC().Format(time.RFC3339)
	endStr := windowEnd.UTC().Format(time.RFC3339)
	err = app.Dao().DB().
		Select("id", "kind", "status", "exec_stage", "owned_by_pod", "started_at", "updated_at", "message", "checkpoint_data", "deployed_spec").
		From("operations").
		Where(dbx.NewExp(
			"target = {:tid} AND started_at >= {:ws} AND started_at <= {:we}",
			dbx.Params{"tid": targetID, "ws": startStr, "we": endStr},
		)).
		OrderBy("started_at ASC").
		All(&opRows)
	if err != nil {
		log.Printf("[INCIDENT_OPS_ERR] target=%s err=%v", targetID, err)
	}
	for _, r := range opRows {
		started, _ := time.Parse(time.RFC3339, r.StartedAt)
		updated, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		incident.Operations = append(incident.Operations, IncidentTimelineOp{
			ID:              r.ID,
			Kind:            r.Kind,
			Status:          r.Status,
			ExecStage:       r.ExecStage,
			OwnedByPod:      r.OwnedByPod,
			StartedAt:       started,
			UpdatedAt:       updated,
			Message:         r.Message,
			HasCheckpoint:   r.CheckpointData != "" && r.CheckpointData != "null",
			HasDeployedSpec: r.DeployedSpec != "" && r.DeployedSpec != "null",
		})
	}

	// Read history within window.
	var histRows []struct {
		Created     string `db:"created"`
		Action      string `db:"action"`
		Status      string `db:"status"`
		OperationID string `db:"operation_id"`
		TriggeredBy string `db:"triggered_by"`
		Message     string `db:"message"`
	}
	err = app.Dao().DB().
		Select("created", "action", "status", "operation_id", "triggered_by", "message").
		From("deployment_history").
		Where(dbx.NewExp(
			"target = {:tid} AND created >= {:ws} AND created <= {:we}",
			dbx.Params{"tid": targetID, "ws": startStr, "we": endStr},
		)).
		OrderBy("created ASC").
		All(&histRows)
	if err != nil {
		log.Printf("[INCIDENT_HIST_ERR] target=%s err=%v", targetID, err)
	}
	for _, r := range histRows {
		at, _ := time.Parse(time.RFC3339, r.Created)
		incident.Events = append(incident.Events, IncidentTimelineEvent{
			At:          at,
			Action:      r.Action,
			Status:      r.Status,
			OperationID: r.OperationID,
			TriggeredBy: r.TriggeredBy,
			Message:     r.Message,
		})
	}

	incident.Summary = summarizeIncidentTimeline(incident)
	return incident
}

// summarizeIncidentTimeline builds a one-paragraph operator-readable summary
// highlighting status transitions, escalations, drift, ambiguity.
//
// Format: discrete clauses separated by "; ". Designed to be readable in
// a UI tooltip or a log line. Never multi-line.
func summarizeIncidentTimeline(inc *IncidentTimeline) string {
	if inc == nil {
		return ""
	}
	var parts []string

	parts = append(parts, fmt.Sprintf("target=%s provider=%s current=%s",
		inc.TargetID, inc.Provider, inc.CurrentStatus))

	// Operations highlights.
	if len(inc.Operations) > 0 {
		var deploys, destroys, rollbacks int
		var succeeded, failed int
		for _, op := range inc.Operations {
			switch op.Kind {
			case "deploy":
				deploys++
			case "destroy":
				destroys++
			case "rollback":
				rollbacks++
			}
			switch op.Status {
			case "succeeded":
				succeeded++
			case "failed":
				failed++
			}
		}
		parts = append(parts, fmt.Sprintf("ops=%d(deploy=%d destroy=%d rollback=%d succeeded=%d failed=%d)",
			len(inc.Operations), deploys, destroys, rollbacks, succeeded, failed))
	}

	// Causality flags in history.
	var causes []string
	seenCauses := map[string]bool{}
	for _, ev := range inc.Events {
		if ev.TriggeredBy != "" && !seenCauses[ev.TriggeredBy] {
			causes = append(causes, ev.TriggeredBy)
			seenCauses[ev.TriggeredBy] = true
		}
	}
	if len(causes) > 0 {
		parts = append(parts, "causes=["+strings.Join(causes, ",")+"]")
	}

	// State flags.
	if inc.Ambiguous {
		parts = append(parts, "currently_ambiguous=true")
	}
	if inc.DriftState != "" && inc.DriftState != "no_drift" && inc.DriftState != "unknown" {
		parts = append(parts, "drift="+inc.DriftState)
	}

	return strings.Join(parts, "; ")
}
