package workers

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// WorkerHeartbeat is an immutable, hashed heartbeat record.
// ReceivedAt is excluded from the hash (content-identity).
type WorkerHeartbeat struct {
	HeartbeatID string `json:"heartbeat_id"`
	WorkerID    string `json:"worker_id"`
	TenantID    string `json:"tenant_id"`
	ActiveTasks int    `json:"active_tasks"`
	ReceivedAt  string `json:"received_at"` // excluded from hash
	HeartbeatHash string `json:"heartbeat_hash"`
}

// NewWorkerHeartbeat creates a hashed heartbeat record.
func NewWorkerHeartbeat(heartbeatID, workerID, tenantID string, activeTasks int) WorkerHeartbeat {
	hb := WorkerHeartbeat{
		HeartbeatID: heartbeatID,
		WorkerID:    workerID,
		TenantID:    tenantID,
		ActiveTasks: activeTasks,
		ReceivedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	hb.HeartbeatHash = computeHeartbeatHash(hb)
	return hb
}

// VerifyHeartbeatHash recomputes the hash and returns (true, "") when it matches.
func VerifyHeartbeatHash(hb WorkerHeartbeat) (bool, string) {
	stored := hb.HeartbeatHash
	hb.HeartbeatHash = ""
	expected := computeHeartbeatHash(hb)
	if stored != expected {
		return false, fmt.Sprintf("heartbeat %q hash mismatch: stored=%s computed=%s",
			hb.HeartbeatID, stored, expected)
	}
	return true, ""
}

// HeartbeatLog is an append-only, thread-safe log of worker heartbeats.
type HeartbeatLog struct {
	mu      sync.RWMutex
	records []WorkerHeartbeat
}

// NewHeartbeatLog returns an empty heartbeat log.
func NewHeartbeatLog() *HeartbeatLog {
	return &HeartbeatLog{}
}

// Record appends a heartbeat. Returns an error on invalid hash.
func (l *HeartbeatLog) Record(hb WorkerHeartbeat) error {
	if ok, reason := VerifyHeartbeatHash(hb); !ok {
		return fmt.Errorf("heartbeat: invalid hash: %s", reason)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, hb)
	return nil
}

// LastHeartbeat returns the most recent heartbeat for a worker, or (WorkerHeartbeat{}, false).
func (l *HeartbeatLog) LastHeartbeat(workerID string) (WorkerHeartbeat, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var last WorkerHeartbeat
	found := false
	for _, hb := range l.records {
		if hb.WorkerID == workerID {
			last = hb
			found = true
		}
	}
	return last, found
}

// IsWorkerAlive returns true when a heartbeat for the worker has been recorded
// more recently than staleSec seconds ago.
func (l *HeartbeatLog) IsWorkerAlive(workerID string, staleSec int) bool {
	hb, ok := l.LastHeartbeat(workerID)
	if !ok {
		return false
	}
	ts, err := time.Parse(time.RFC3339Nano, hb.ReceivedAt)
	if err != nil {
		return false
	}
	return time.Since(ts).Seconds() < float64(staleSec)
}

// AllForWorker returns all heartbeats for a worker in append order.
func (l *HeartbeatLog) AllForWorker(workerID string) []WorkerHeartbeat {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out []WorkerHeartbeat
	for _, hb := range l.records {
		if hb.WorkerID == workerID {
			out = append(out, hb)
		}
	}
	return out
}

func computeHeartbeatHash(hb WorkerHeartbeat) string {
	h := sha256.New()
	fmt.Fprintf(h, "heartbeat:%s|worker:%s|tenant:%s|active:%d",
		hb.HeartbeatID, hb.WorkerID, hb.TenantID, hb.ActiveTasks)
	return fmt.Sprintf("%x", h.Sum(nil))
}
