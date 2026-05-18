package topology

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// ExportTopologySnapshot produces a tamper-evident topology snapshot.
// GeneratedAt and TopologyHash are excluded from the hash for content-identity hashing.
func ExportTopologySnapshot(
	graph TopologyGraph,
	capabilities []ProviderCapabilities,
	failureDomains []FailureDomain,
	deps []CrossProviderDependency,
) *TopologySnapshot {
	snap := &TopologySnapshot{
		SchemaVersion:  TopologySnapshotSchemaVersion,
		ExecutionID:    graph.ExecutionID,
		Graph:          graph,
		Capabilities:   append([]ProviderCapabilities(nil), capabilities...),
		FailureDomains: append([]FailureDomain(nil), failureDomains...),
		Dependencies:   append([]CrossProviderDependency(nil), deps...),
		Limitations:    topologyLimitations(),
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	snap.TopologyHash = computeTopologyHash(snap)
	return snap
}

// VerifyTopologySnapshot re-derives the hash and checks it against TopologyHash.
func VerifyTopologySnapshot(snap *TopologySnapshot) (bool, string) {
	if snap == nil {
		return false, "snapshot is nil"
	}
	if snap.TopologyHash == "" {
		return false, "TopologyHash is empty"
	}
	recomputed := computeTopologyHash(snap)
	if recomputed != snap.TopologyHash {
		return false, fmt.Sprintf("TopologyHash mismatch: stored=%q recomputed=%q", snap.TopologyHash, recomputed)
	}
	return true, ""
}

func computeTopologyHash(snap *TopologySnapshot) string {
	canonical := struct {
		SchemaVersion  string                    `json:"schema_version"`
		ExecutionID    string                    `json:"execution_id"`
		Graph          TopologyGraph             `json:"graph"`
		Capabilities   []ProviderCapabilities    `json:"capabilities"`
		FailureDomains []FailureDomain           `json:"failure_domains"`
		Dependencies   []CrossProviderDependency `json:"dependencies"`
	}{
		SchemaVersion:  snap.SchemaVersion,
		ExecutionID:    snap.ExecutionID,
		Graph:          snap.Graph,
		Capabilities:   snap.Capabilities,
		FailureDomains: snap.FailureDomains,
		Dependencies:   snap.Dependencies,
	}
	b, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}
