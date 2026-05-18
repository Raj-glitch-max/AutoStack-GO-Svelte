package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/janlauber/one-click/pkg/workers"
)

// WorkerPersistenceStore persists worker ownership and heartbeat records.
type WorkerPersistenceStore struct {
	kv DurableKVStore
}

// NewWorkerPersistenceStore wraps a KV store for worker persistence.
func NewWorkerPersistenceStore(kv DurableKVStore) *WorkerPersistenceStore {
	return &WorkerPersistenceStore{kv: kv}
}

// SaveWorker persists a Worker descriptor.
func (s *WorkerPersistenceStore) SaveWorker(ctx context.Context, w workers.Worker) error {
	if ok, reason := workers.VerifyWorkerHash(w); !ok {
		return fmt.Errorf("worker store: invalid hash: %s", reason)
	}
	b, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("worker store: marshal: %w", err)
	}
	return s.kv.Put(ctx, "worker:"+w.WorkerID, b)
}

// LoadWorker retrieves a worker by ID.
func (s *WorkerPersistenceStore) LoadWorker(ctx context.Context, workerID string) (workers.Worker, bool, error) {
	b, ok, err := s.kv.Get(ctx, "worker:"+workerID)
	if err != nil || !ok {
		return workers.Worker{}, ok, err
	}
	var w workers.Worker
	if err := json.Unmarshal(b, &w); err != nil {
		return workers.Worker{}, false, err
	}
	return w, true, nil
}

// AllWorkers returns all persisted workers.
func (s *WorkerPersistenceStore) AllWorkers(ctx context.Context) ([]workers.Worker, error) {
	all, err := s.kv.List(ctx, "worker:")
	if err != nil {
		return nil, err
	}
	out := make([]workers.Worker, 0, len(all))
	for _, b := range all {
		var w workers.Worker
		if err := json.Unmarshal(b, &w); err != nil {
			continue
		}
		out = append(out, w)
	}
	return out, nil
}

// SaveOwnershipRecord persists an ownership record.
func (s *WorkerPersistenceStore) SaveOwnershipRecord(ctx context.Context, r workers.OwnershipRecord) error {
	if ok, reason := workers.VerifyOwnershipRecord(r); !ok {
		return fmt.Errorf("worker store: invalid ownership hash: %s", reason)
	}
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("worker store: ownership marshal: %w", err)
	}
	return s.kv.Put(ctx, "ownership:"+r.RecordID, b)
}

// AllOwnershipRecords returns all ownership records for a taskID.
func (s *WorkerPersistenceStore) AllOwnershipRecords(ctx context.Context, taskID string) ([]workers.OwnershipRecord, error) {
	all, err := s.kv.List(ctx, "ownership:")
	if err != nil {
		return nil, err
	}
	var out []workers.OwnershipRecord
	for _, b := range all {
		var r workers.OwnershipRecord
		if err := json.Unmarshal(b, &r); err != nil {
			continue
		}
		if r.TaskID == taskID {
			out = append(out, r)
		}
	}
	return out, nil
}

// SaveHeartbeat persists a worker heartbeat.
func (s *WorkerPersistenceStore) SaveHeartbeat(ctx context.Context, hb workers.WorkerHeartbeat) error {
	if ok, reason := workers.VerifyHeartbeatHash(hb); !ok {
		return fmt.Errorf("worker store: invalid heartbeat hash: %s", reason)
	}
	b, err := json.Marshal(hb)
	if err != nil {
		return fmt.Errorf("worker store: heartbeat marshal: %w", err)
	}
	return s.kv.Put(ctx, "heartbeat:"+hb.HeartbeatID, b)
}
