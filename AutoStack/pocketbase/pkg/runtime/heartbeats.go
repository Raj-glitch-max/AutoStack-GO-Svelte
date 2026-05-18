package runtime

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// RuntimeHeartbeat captures a node's liveness and resource pressure at one clock position.
// Heartbeats are immutable once created.
type RuntimeHeartbeat struct {
	NodeID              string  `json:"node_id"`
	TenantID            string  `json:"tenant_id"`
	Timestamp           string  `json:"timestamp"`
	LogicalClock        int64   `json:"logical_clock"`
	SchedulerPressure   float64 `json:"scheduler_pressure"`   // 0.0 (idle) – 1.0 (saturated)
	ReplayBacklog       int64   `json:"replay_backlog"`
	ArchiveBacklog      int64   `json:"archive_backlog"`
	VerificationBacklog int64   `json:"verification_backlog"`
	ActiveLeaseCount    int64   `json:"active_lease_count"`
	HeartbeatHash       string  `json:"heartbeat_hash"`
}

// NewHeartbeat creates an immutable RuntimeHeartbeat and computes its content hash.
func NewHeartbeat(
	nodeID string,
	tenantID string,
	clock int64,
	schedulerPressure float64,
	replayBacklog int64,
	archiveBacklog int64,
	verificationBacklog int64,
	activeLeaseCount int64,
) RuntimeHeartbeat {
	hb := RuntimeHeartbeat{
		NodeID:              nodeID,
		TenantID:            tenantID,
		Timestamp:           time.Now().UTC().Format(time.RFC3339Nano),
		LogicalClock:        clock,
		SchedulerPressure:   schedulerPressure,
		ReplayBacklog:       replayBacklog,
		ArchiveBacklog:      archiveBacklog,
		VerificationBacklog: verificationBacklog,
		ActiveLeaseCount:    activeLeaseCount,
	}
	hb.HeartbeatHash = computeHeartbeatHash(hb)
	return hb
}

// CompareHeartbeats provides a total order: LogicalClock → Timestamp lexical → NodeID lexical.
// Returns negative if a < b, zero if equal, positive if a > b.
func CompareHeartbeats(a, b RuntimeHeartbeat) int {
	if a.LogicalClock != b.LogicalClock {
		if a.LogicalClock < b.LogicalClock {
			return -1
		}
		return 1
	}
	if a.Timestamp != b.Timestamp {
		if a.Timestamp < b.Timestamp {
			return -1
		}
		return 1
	}
	if a.NodeID < b.NodeID {
		return -1
	}
	if a.NodeID > b.NodeID {
		return 1
	}
	return 0
}

// IsHeartbeatStale returns true when (nowClock - hb.LogicalClock) > maxStaleClock.
func IsHeartbeatStale(hb RuntimeHeartbeat, nowClock int64, maxStaleClock int64) bool {
	return nowClock-hb.LogicalClock > maxStaleClock
}

// LatestHeartbeat returns the heartbeat with the highest LogicalClock.
// Returns the zero value when the slice is empty.
func LatestHeartbeat(hbs []RuntimeHeartbeat) RuntimeHeartbeat {
	if len(hbs) == 0 {
		return RuntimeHeartbeat{}
	}
	latest := hbs[0]
	for _, hb := range hbs[1:] {
		if CompareHeartbeats(hb, latest) > 0 {
			latest = hb
		}
	}
	return latest
}

// HeartbeatsForNode filters a slice to only the heartbeats from the given node.
func HeartbeatsForNode(hbs []RuntimeHeartbeat, nodeID string) []RuntimeHeartbeat {
	var result []RuntimeHeartbeat
	for _, hb := range hbs {
		if hb.NodeID == nodeID {
			result = append(result, hb)
		}
	}
	return result
}

// SortHeartbeats returns a new slice sorted in ascending LogicalClock order.
// The original slice is not modified.
func SortHeartbeats(hbs []RuntimeHeartbeat) []RuntimeHeartbeat {
	out := make([]RuntimeHeartbeat, len(hbs))
	copy(out, hbs)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && CompareHeartbeats(out[j], out[j-1]) < 0; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// VerifyHeartbeatHash checks whether the stored hash matches the recomputed hash.
func VerifyHeartbeatHash(hb RuntimeHeartbeat) (bool, string) {
	stored := hb.HeartbeatHash
	hb.HeartbeatHash = ""
	expected := computeHeartbeatHash(hb)
	if stored != expected {
		return false, fmt.Sprintf("heartbeat hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

func computeHeartbeatHash(hb RuntimeHeartbeat) string {
	h := sha256.New()
	fmt.Fprintf(h, "node:%s|tenant:%s|ts:%s|clock:%d|pressure:%.6f|replay:%d|archive:%d|verify:%d|leases:%d",
		hb.NodeID, hb.TenantID, hb.Timestamp, hb.LogicalClock,
		hb.SchedulerPressure, hb.ReplayBacklog, hb.ArchiveBacklog,
		hb.VerificationBacklog, hb.ActiveLeaseCount,
	)
	return fmt.Sprintf("%x", h.Sum(nil))
}
