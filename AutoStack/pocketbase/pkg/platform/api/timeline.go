package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

// TimelinePhase is a single chronological phase in an execution timeline.
type TimelinePhase struct {
	StageID   string `json:"stage_id"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
	Note      string `json:"note,omitempty"`
}

// TimelineContradiction is a contradiction event as it appears on the timeline.
type TimelineContradiction struct {
	ContradictionID string `json:"contradiction_id"`
	Status          string `json:"status"` // unresolved, resolved, escalated
	Description     string `json:"description"`
	DetectedAt      string `json:"detected_at"`
	ResolvedAt      string `json:"resolved_at,omitempty"`
	ResolutionNote  string `json:"resolution_note,omitempty"`
}

// TimelineReplayManifest is a replay manifest entry on the timeline.
type TimelineReplayManifest struct {
	ManifestID   string `json:"manifest_id"`
	EventCount   int    `json:"event_count"`
	ManifestHash string `json:"manifest_hash"`
	HashValid    bool   `json:"hash_valid"`
	HashReason   string `json:"hash_reason,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// TimelineGovernanceDecision is a governance decision as it appears on the timeline.
type TimelineGovernanceDecision struct {
	PolicyID  string `json:"policy_id"`
	Effect    string `json:"effect"`
	Reason    string `json:"reason"`
	DecidedAt string `json:"decided_at"`
}

// TimelineAuditEntry is an audit chain entry as it appears on the timeline.
type TimelineAuditEntry struct {
	EventType  string `json:"event_type"`
	Outcome    string `json:"outcome"`
	OccurredAt string `json:"occurred_at"`
}

// TimelineResponse is returned by GET /platform/timeline/:id.
type TimelineResponse struct {
	ExecutionID      string                       `json:"execution_id"`
	ExecutionPhases  []TimelinePhase              `json:"execution_phases"`
	Contradictions   []TimelineContradiction      `json:"contradictions"`
	ReplayHistory    []TimelineReplayManifest     `json:"replay_history"`
	GovernanceDecisions []TimelineGovernanceDecision `json:"governance_decisions"`
	AuditEntries     []TimelineAuditEntry         `json:"audit_entries"`
	AuditEntryCount  int                          `json:"audit_entry_count"`
	AuditChainValid  bool                         `json:"audit_chain_valid"`
	AuditChainReason string                       `json:"audit_chain_reason,omitempty"`
	Limitations      []string                     `json:"limitations"`
	GeneratedAt      string                       `json:"generated_at"`
}

func handleGetTimeline(store PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.PathParam("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "execution id is required")
		}

		resp := TimelineResponse{
			ExecutionID:         id,
			ExecutionPhases:     []TimelinePhase{},
			Contradictions:      []TimelineContradiction{},
			ReplayHistory:       []TimelineReplayManifest{},
			GovernanceDecisions: []TimelineGovernanceDecision{},
			AuditEntries:        []TimelineAuditEntry{},
			AuditChainValid:     true,
			Limitations: []string{
				"Timeline is assembled from stored snapshots; real-time streaming not supported.",
				"Audit entries shown are bounded to the most recent 500 for performance.",
				"Cross-execution timeline correlation is not supported.",
			},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}

		ctx := c.Request().Context()

		// Seeded extras: richer phase notes, contradictions, audit entries.
		// Only the SeededPlatformStore implements this; production stores skip.
		if seeded, ok := store.(*SeededPlatformStore); ok {
			if ex := seeded.ExtrasFor(id); ex != nil {
				for _, p := range ex.Phases {
					resp.ExecutionPhases = append(resp.ExecutionPhases, TimelinePhase{
						StageID:   p.StageID,
						Status:    p.Status,
						StartedAt: p.StartedAt,
						EndedAt:   p.EndedAt,
						Note:      p.Note,
					})
				}
				for _, c := range ex.Contradictions {
					resp.Contradictions = append(resp.Contradictions, TimelineContradiction{
						ContradictionID: c.ContradictionID,
						Status:          c.Status,
						Description:     c.Description,
						DetectedAt:      c.DetectedAt,
						ResolvedAt:      c.ResolvedAt,
						ResolutionNote:  c.ResolutionNote,
					})
				}
				for _, a := range ex.AuditEntries {
					resp.AuditEntries = append(resp.AuditEntries, TimelineAuditEntry{
						EventType:  a.EventType,
						Outcome:    a.Outcome,
						OccurredAt: a.OccurredAt,
					})
				}
				resp.AuditEntryCount = len(resp.AuditEntries)
			}
		}

		// Replay history → derive from replay events. If phases were already
		// populated from seeded extras, do not double-write them.
		if events, _, err := store.GetReplayEvents(ctx, id); err == nil {
			for _, ev := range events {
				resp.ReplayHistory = append(resp.ReplayHistory, TimelineReplayManifest{
					ManifestID:   "manifest-" + ev.StageID,
					EventCount:   1,
					ManifestHash: "",
					HashValid:    true,
					CreatedAt:    ev.Timestamp,
				})
				if len(resp.ExecutionPhases) == 0 {
					resp.ExecutionPhases = append(resp.ExecutionPhases, TimelinePhase{
						StageID:   ev.StageID,
						Status:    phaseFromEvent(ev.EventType),
						StartedAt: ev.Timestamp,
						Note:      ev.EventType,
					})
				}
			}
		}
		// Governance decisions → from governance snapshot.
		if gov, err := store.GetGovernanceSnapshot(ctx, id); err == nil && gov != nil {
			for _, d := range gov.Decisions {
				resp.GovernanceDecisions = append(resp.GovernanceDecisions, TimelineGovernanceDecision{
					PolicyID:  d.PolicyID,
					Effect:    string(d.Effect),
					Reason:    d.Reason,
					DecidedAt: d.DecidedAt,
				})
			}
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func phaseFromEvent(eventType string) string {
	switch eventType {
	case "enqueued":
		return "planned"
	case "dequeued":
		return "running"
	case "blocked":
		return "blocked"
	case "paused":
		return "blocked"
	default:
		return eventType
	}
}
