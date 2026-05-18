package replay

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// BuildReplayManifest constructs a tamper-evident manifest of replay checkpoints.
// ManifestHash is computed over all stable fields (GeneratedAt excluded).
func BuildReplayManifest(
	manifestID, executionID, tenantID string,
	checkpoints []ReplayCheckpoint,
	eventCount int,
) ReplayManifest {
	cpCopy := append([]ReplayCheckpoint(nil), checkpoints...)
	m := ReplayManifest{
		ManifestID:    manifestID,
		ExecutionID:   executionID,
		TenantID:      tenantID,
		SchemaVersion: ReplaySchemaVersion,
		Checkpoints:   cpCopy,
		EventCount:    eventCount,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	m.ManifestHash = computeManifestHash(m)
	return m
}

// VerifyReplayManifest recomputes ManifestHash and checks it matches.
func VerifyReplayManifest(manifest ReplayManifest) (bool, string) {
	if manifest.ManifestHash == "" {
		return false, "ManifestHash is empty"
	}
	expected := computeManifestHash(manifest)
	if manifest.ManifestHash != expected {
		return false, fmt.Sprintf("ManifestHash mismatch: expected %q got %q", expected, manifest.ManifestHash)
	}
	return true, ""
}

func computeManifestHash(m ReplayManifest) string {
	h := sha256.New()
	fmt.Fprintf(h, "manifest_id:%s|exec:%s|tenant:%s|schema:%s|events:%d|",
		m.ManifestID, m.ExecutionID, m.TenantID, m.SchemaVersion, m.EventCount)
	for _, cp := range m.Checkpoints {
		fmt.Fprintf(h, "checkpoint:%s:%d:%s|", cp.CheckpointID, cp.AtClock, cp.StateHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
