package api

import (
	"context"
	"fmt"

	"github.com/janlauber/one-click/pkg/governance"
	"github.com/janlauber/one-click/pkg/scheduler"
	"github.com/janlauber/one-click/pkg/topology"
	"github.com/janlauber/one-click/pkg/verifier"
)

// PlatformStore is the read-only data access interface for the platform API
// handlers. Implementations are provided by the host application (PocketBase
// backend) and may read from persistent storage, in-memory snapshots, or both.
//
// All methods must be safe for concurrent use.
type PlatformStore interface {
	// ListExecutionSummaries returns a summary row for every known execution.
	ListExecutionSummaries(ctx context.Context) ([]ExecutionSummary, error)

	// GetExecutionDetail returns the assembled detail view for a single execution.
	GetExecutionDetail(ctx context.Context, executionID string) (*ExecutionDetailResponse, error)

	// GetTopologySnapshot returns the stored topology snapshot for an execution.
	GetTopologySnapshot(ctx context.Context, executionID string) (*topology.TopologySnapshot, error)

	// GetSchedulingSnapshot returns the stored scheduling snapshot for an execution.
	GetSchedulingSnapshot(ctx context.Context, executionID string) (*scheduler.SchedulingSnapshot, error)

	// GetGovernanceSnapshot returns the stored governance snapshot for an execution.
	GetGovernanceSnapshot(ctx context.Context, executionID string) (*governance.GovernanceSnapshot, error)

	// GetReplayEvents returns the ordered replay event list for an execution,
	// plus the derived replay order (stage IDs in first-enqueue order).
	GetReplayEvents(ctx context.Context, executionID string) ([]scheduler.SchedulerReplayEvent, []string, error)
}

// verifySchedulingSnapshot recomputes the scheduler hash and checks it matches.
func verifySchedulingSnapshot(snap *scheduler.SchedulingSnapshot) (bool, string) {
	if snap == nil {
		return false, "snapshot is nil"
	}
	ok, reason := scheduler.VerifySchedulingSnapshotHash(snap)
	if !ok {
		return false, reason
	}
	return true, ""
}

// NoopPlatformStore is a zero-dependency store implementation that returns
// "not found" for every call. Useful for tests and for wiring up the API
// before the real store is implemented.
type NoopPlatformStore struct{}

func (NoopPlatformStore) ListExecutionSummaries(_ context.Context) ([]ExecutionSummary, error) {
	return nil, nil
}

func (NoopPlatformStore) GetExecutionDetail(_ context.Context, id string) (*ExecutionDetailResponse, error) {
	return nil, fmt.Errorf("execution %q not found", id)
}

func (NoopPlatformStore) GetTopologySnapshot(_ context.Context, id string) (*topology.TopologySnapshot, error) {
	return nil, fmt.Errorf("topology snapshot for %q not found", id)
}

func (NoopPlatformStore) GetSchedulingSnapshot(_ context.Context, id string) (*scheduler.SchedulingSnapshot, error) {
	return nil, fmt.Errorf("scheduling snapshot for %q not found", id)
}

func (NoopPlatformStore) GetGovernanceSnapshot(_ context.Context, id string) (*governance.GovernanceSnapshot, error) {
	return nil, fmt.Errorf("governance snapshot for %q not found", id)
}

func (NoopPlatformStore) GetReplayEvents(_ context.Context, id string) ([]scheduler.SchedulerReplayEvent, []string, error) {
	return nil, nil, fmt.Errorf("replay events for %q not found", id)
}

// InMemoryPlatformStore is a test-friendly store backed by in-memory maps.
// Thread safety is the caller's responsibility.
type InMemoryPlatformStore struct {
	Summaries    []ExecutionSummary
	Details      map[string]*ExecutionDetailResponse
	Topologies   map[string]*topology.TopologySnapshot
	Schedules    map[string]*scheduler.SchedulingSnapshot
	Governances  map[string]*governance.GovernanceSnapshot
	ReplayEvents map[string][]scheduler.SchedulerReplayEvent
	ReplayOrders map[string][]string

	verificationSnaps map[string]*verifier.VerificationSnapshot
}

// NewInMemoryPlatformStore returns an empty InMemoryPlatformStore.
func NewInMemoryPlatformStore() *InMemoryPlatformStore {
	return &InMemoryPlatformStore{
		Details:      make(map[string]*ExecutionDetailResponse),
		Topologies:   make(map[string]*topology.TopologySnapshot),
		Schedules:    make(map[string]*scheduler.SchedulingSnapshot),
		Governances:  make(map[string]*governance.GovernanceSnapshot),
		ReplayEvents: make(map[string][]scheduler.SchedulerReplayEvent),
		ReplayOrders: make(map[string][]string),
	}
}

func (s *InMemoryPlatformStore) ListExecutionSummaries(_ context.Context) ([]ExecutionSummary, error) {
	return append([]ExecutionSummary(nil), s.Summaries...), nil
}

func (s *InMemoryPlatformStore) GetExecutionDetail(_ context.Context, id string) (*ExecutionDetailResponse, error) {
	d, ok := s.Details[id]
	if !ok {
		return nil, fmt.Errorf("execution %q not found", id)
	}
	return d, nil
}

func (s *InMemoryPlatformStore) GetTopologySnapshot(_ context.Context, id string) (*topology.TopologySnapshot, error) {
	snap, ok := s.Topologies[id]
	if !ok {
		return nil, fmt.Errorf("topology snapshot for %q not found", id)
	}
	return snap, nil
}

func (s *InMemoryPlatformStore) GetSchedulingSnapshot(_ context.Context, id string) (*scheduler.SchedulingSnapshot, error) {
	snap, ok := s.Schedules[id]
	if !ok {
		return nil, fmt.Errorf("scheduling snapshot for %q not found", id)
	}
	return snap, nil
}

func (s *InMemoryPlatformStore) GetGovernanceSnapshot(_ context.Context, id string) (*governance.GovernanceSnapshot, error) {
	snap, ok := s.Governances[id]
	if !ok {
		return nil, fmt.Errorf("governance snapshot for %q not found", id)
	}
	return snap, nil
}

func (s *InMemoryPlatformStore) GetReplayEvents(_ context.Context, id string) ([]scheduler.SchedulerReplayEvent, []string, error) {
	events, ok := s.ReplayEvents[id]
	if !ok {
		return nil, nil, fmt.Errorf("replay events for %q not found", id)
	}
	order := s.ReplayOrders[id]
	return append([]scheduler.SchedulerReplayEvent(nil), events...), append([]string(nil), order...), nil
}
