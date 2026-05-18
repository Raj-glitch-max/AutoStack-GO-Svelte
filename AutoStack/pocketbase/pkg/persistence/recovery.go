package persistence

import (
	"context"
	"fmt"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/workers"
)

// PersistentRuntimeState is the recovered runtime state from durable storage.
type PersistentRuntimeState struct {
	ExecutionID      string
	TenantID         string
	Tasks            []executor.ExecutionTask
	LatestCheckpoint *executor.ExecutionCheckpoint
	Transitions      []workers.OwnershipRecord
	Certifications   []audit.ReplayCertificationReport
	ClockContinuity  bool   // true when logical clock is unbroken
	LastKnownClock   int64
}

// RecoveryReport summarizes the outcome of a recovery attempt.
type RecoveryReport struct {
	ExecutionID        string `json:"execution_id"`
	TenantID           string `json:"tenant_id"`
	TasksRecovered     int    `json:"tasks_recovered"`
	CheckpointRestored bool   `json:"checkpoint_restored"`
	ClockContinuous    bool   `json:"clock_continuous"`
	CertificationsLoaded int  `json:"certifications_loaded"`
	Errors             []string `json:"errors,omitempty"`
}

// RecoverPersistentRuntime rebuilds runtime state from durable storage.
// It verifies integrity at each step and records errors without halting.
// Clock continuity is validated by checking that the checkpoint's AtClock
// is <= the maximum task LogicalClock found in the store.
func RecoverPersistentRuntime(
	ctx context.Context,
	executionID, tenantID, checkpointID string,
	taskStore *ExecutionTaskStore,
	snapshotStore *SnapshotPersistenceStore,
) (PersistentRuntimeState, RecoveryReport) {
	state := PersistentRuntimeState{
		ExecutionID: executionID,
		TenantID:    tenantID,
	}
	report := RecoveryReport{
		ExecutionID: executionID,
		TenantID:    tenantID,
	}

	// Recover tasks.
	tasks, err := taskStore.ListForExecution(ctx, executionID)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("task recovery: %v", err))
	} else {
		// Verify each task hash.
		for _, t := range tasks {
			if ok, reason := executor.VerifyTaskHash(t); !ok {
				report.Errors = append(report.Errors, fmt.Sprintf("task %q hash invalid: %s", t.TaskID, reason))
				continue
			}
			state.Tasks = append(state.Tasks, t)
		}
		report.TasksRecovered = len(state.Tasks)
	}

	// Find max logical clock among recovered tasks.
	var maxClock int64
	for _, t := range state.Tasks {
		if t.LogicalClock > maxClock {
			maxClock = t.LogicalClock
		}
	}
	state.LastKnownClock = maxClock

	// Recover checkpoint.
	if checkpointID != "" {
		cp, found, err := taskStore.LoadCheckpoint(ctx, checkpointID)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("checkpoint load: %v", err))
		} else if found {
			if ok, reason := executor.VerifyExecutionCheckpoint(cp); !ok {
				report.Errors = append(report.Errors, fmt.Sprintf("checkpoint hash invalid: %s", reason))
			} else {
				state.LatestCheckpoint = &cp
				report.CheckpointRestored = true
				// Clock continuity: checkpoint's AtClock must not exceed what we see in tasks.
				state.ClockContinuity = cp.AtClock <= maxClock || maxClock == 0
			}
		}
	} else {
		state.ClockContinuity = true
	}
	report.ClockContinuous = state.ClockContinuity

	// Recover certifications.
	if snapshotStore != nil {
		certs, err := snapshotStore.AllCertificationsForExecution(ctx, executionID)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("certification recovery: %v", err))
		} else {
			for _, cert := range certs {
				if ok, reason := audit.VerifyCertificationHash(cert); !ok {
					report.Errors = append(report.Errors, fmt.Sprintf("certification %q hash invalid: %s", cert.CertificationID, reason))
					continue
				}
				state.Certifications = append(state.Certifications, cert)
			}
			report.CertificationsLoaded = len(state.Certifications)
		}
	}

	return state, report
}
