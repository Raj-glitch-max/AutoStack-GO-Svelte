package workers

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// WorkerSnapshot is a tamper-evident point-in-time capture of all registered workers.
// SnapshotAt is excluded from SnapshotHash (content-identity).
type WorkerSnapshot struct {
	SnapshotID   string   `json:"snapshot_id"`
	TenantID     string   `json:"tenant_id"`
	Workers      []Worker `json:"workers"`
	TotalCount   int      `json:"total_count"`
	OnlineCount  int      `json:"online_count"`
	SnapshotAt   string   `json:"snapshot_at"` // excluded from hash
	SnapshotHash string   `json:"snapshot_hash"`
}

// BuildWorkerSnapshot captures all provided workers into a tamper-evident snapshot.
func BuildWorkerSnapshot(snapshotID, tenantID string, workers []Worker) WorkerSnapshot {
	wCopy := make([]Worker, len(workers))
	copy(wCopy, workers)
	online := 0
	for _, w := range wCopy {
		if w.State == WorkerStateOnline {
			online++
		}
	}
	snap := WorkerSnapshot{
		SnapshotID:  snapshotID,
		TenantID:    tenantID,
		Workers:     wCopy,
		TotalCount:  len(wCopy),
		OnlineCount: online,
		SnapshotAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeWorkerSnapshotHash(snap)
	return snap
}

// VerifyWorkerSnapshot recomputes the hash and returns (true, "") when it matches.
func VerifyWorkerSnapshot(snap WorkerSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeWorkerSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("worker snapshot %q hash mismatch: stored=%s computed=%s",
			snap.SnapshotID, stored, expected)
	}
	return true, ""
}

// RecoverWorkerAssignments scans the ownership history for tasks currently assigned
// to workers that are not in the liveWorkers set. Returns evidence for each
// potential re-assignment; callers decide what action to take.
func RecoverWorkerAssignments(history *OwnershipHistory, liveWorkers map[string]struct{}, taskIDs []string) []OwnershipRecord {
	var out []OwnershipRecord
	for _, taskID := range taskIDs {
		current := history.CurrentOwner(taskID)
		if current == "" {
			continue
		}
		if _, alive := liveWorkers[current]; !alive {
			recs := history.ForTask(taskID)
			if len(recs) > 0 {
				out = append(out, recs[len(recs)-1])
			}
		}
	}
	return out
}

func computeWorkerSnapshotHash(snap WorkerSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "workersnap:%s|tenant:%s|total:%d|online:%d",
		snap.SnapshotID, snap.TenantID, snap.TotalCount, snap.OnlineCount)
	for _, w := range snap.Workers {
		fmt.Fprintf(h, "|w:%s/%s/%s", w.WorkerID, w.State, w.WorkerHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
