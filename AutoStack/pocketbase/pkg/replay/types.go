// Package replay implements replay engine v2: deterministic state reconstruction
// from ordered event sequences, with checkpoint-assisted partial replay and
// divergence detection.
//
// Replay is strictly read-only. No function in this package mutates any event,
// stream, or execution state.
package replay

import "github.com/janlauber/one-click/pkg/events"

const ReplaySchemaVersion = "5.0.0"

// ReplayCheckpoint marks a known-good state at a specific logical clock position.
// Checkpoints reduce replay cost — a partial replay can start from a checkpoint
// instead of position 0.
type ReplayCheckpoint struct {
	CheckpointID string `json:"checkpoint_id"`
	StreamID     string `json:"stream_id"`
	ExecutionID  string `json:"execution_id"`
	TenantID     string `json:"tenant_id"`
	AtClock      int64  `json:"at_clock"`
	AtTimestamp  string `json:"at_timestamp"`
	// StateHash is SHA-256 over the hashes of all events up to and including AtClock.
	StateHash string `json:"state_hash"`
	CreatedAt string `json:"created_at"`
}

// ReplayResult is the output of a replay operation.
// It is read-only evidence — it never triggers any execution action.
type ReplayResult struct {
	ExecutionID  string                  `json:"execution_id"`
	TenantID     string                  `json:"tenant_id"`
	Events       []events.PlatformEvent  `json:"events"`
	Checkpoints  []ReplayCheckpoint      `json:"checkpoints"`
	FromClock    int64                   `json:"from_clock"`
	ToClock      int64                   `json:"to_clock"`
	EventCount   int                     `json:"event_count"`
	// Diverged is true when the replayed event sequence differs from an original.
	Diverged     bool  `json:"diverged"`
	DivergenceAt int64 `json:"divergence_at,omitempty"`
	Limitations  []string                `json:"limitations"`
	ReplayedAt   string                  `json:"replayed_at"`
}

// ReplayManifest is a tamper-evident index of replay checkpoints for an execution.
type ReplayManifest struct {
	ManifestID    string             `json:"manifest_id"`
	ExecutionID   string             `json:"execution_id"`
	TenantID      string             `json:"tenant_id"`
	SchemaVersion string             `json:"schema_version"`
	Checkpoints   []ReplayCheckpoint `json:"checkpoints"`
	EventCount    int                `json:"event_count"`
	ManifestHash  string             `json:"manifest_hash"`
	GeneratedAt   string             `json:"generated_at"`
}

func replayLimitations() []string {
	return []string{
		"Replay is read-only. It does not re-execute any stage or trigger any provider action.",
		"Partial replay starts from the nearest checkpoint before fromClock — events before the checkpoint are excluded from the result.",
		"Divergence detection compares event hashes at each position — different ordering policies will produce divergence false-positives.",
		"Snapshot-assisted replay requires the checkpoint StateHash to have been computed over the same event set.",
	}
}
