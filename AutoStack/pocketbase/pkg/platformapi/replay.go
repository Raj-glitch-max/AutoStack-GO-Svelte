package platformapi

import (
	"time"

	"github.com/janlauber/one-click/pkg/replay"
)

// ReplayView is a read-only projection of a ReplayResult safe for API responses.
type ReplayView struct {
	ExecutionID  string `json:"execution_id"`
	TenantID     string `json:"tenant_id"`
	EventCount   int    `json:"event_count"`
	FromClock    int64  `json:"from_clock"`
	ToClock      int64  `json:"to_clock"`
	Diverged     bool   `json:"diverged"`
	DivergenceAt int64  `json:"divergence_at,omitempty"`
	ReplayedAt   string `json:"replayed_at"`
	Limitations  []string `json:"limitations"`
}

// ReplayManifestView is a read-only projection of a ReplayManifest.
type ReplayManifestView struct {
	ManifestID      string `json:"manifest_id"`
	ExecutionID     string `json:"execution_id"`
	TenantID        string `json:"tenant_id"`
	CheckpointCount int    `json:"checkpoint_count"`
	EventCount      int    `json:"event_count"`
	HashValid       bool   `json:"hash_valid"`
	GeneratedAt     string `json:"generated_at"`
}

// ReplayQueryResult groups replay views.
type ReplayQueryResult struct {
	Views    []ReplayView `json:"views"`
	Manifests []ReplayManifestView `json:"manifests"`
	QueryAt  string       `json:"query_at"`
}

// ProjectReplayResult converts a ReplayResult to a read-only API view.
func ProjectReplayResult(r replay.ReplayResult) ReplayView {
	lims := make([]string, len(r.Limitations))
	copy(lims, r.Limitations)
	return ReplayView{
		ExecutionID:  r.ExecutionID,
		TenantID:     r.TenantID,
		EventCount:   r.EventCount,
		FromClock:    r.FromClock,
		ToClock:      r.ToClock,
		Diverged:     r.Diverged,
		DivergenceAt: r.DivergenceAt,
		ReplayedAt:   r.ReplayedAt,
		Limitations:  lims,
	}
}

// ProjectReplayManifest converts a ReplayManifest to a read-only API view.
func ProjectReplayManifest(m replay.ReplayManifest) ReplayManifestView {
	ok, _ := replay.VerifyReplayManifest(m)
	return ReplayManifestView{
		ManifestID:      m.ManifestID,
		ExecutionID:     m.ExecutionID,
		TenantID:        m.TenantID,
		CheckpointCount: len(m.Checkpoints),
		EventCount:      m.EventCount,
		HashValid:       ok,
		GeneratedAt:     m.GeneratedAt,
	}
}

// QueryReplayState builds a combined view of replay results and manifests
// for a given executionID and tenantID.
func QueryReplayState(results []replay.ReplayResult, manifests []replay.ReplayManifest, executionID, tenantID string) ReplayQueryResult {
	q := ReplayQueryResult{
		QueryAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, r := range results {
		if r.ExecutionID != executionID || r.TenantID != tenantID {
			continue
		}
		q.Views = append(q.Views, ProjectReplayResult(r))
	}
	for _, m := range manifests {
		if m.ExecutionID != executionID || m.TenantID != tenantID {
			continue
		}
		q.Manifests = append(q.Manifests, ProjectReplayManifest(m))
	}
	return q
}
