package executor

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// StageRunner executes a single ExecutionStage.
//
// Rules:
//   - Nodes are executed in deterministic order (alphabetical by NodeID).
//   - Provider execution is serialized — no opportunistic parallelism.
//   - Execution stops immediately on ambiguity or contract violation.
//   - Journal entries are written BEFORE and AFTER every provider action.
type StageRunner struct {
	provider ProviderExecutor
}

func NewStageRunner(provider ProviderExecutor) *StageRunner {
	if provider == nil {
		panic("executor: StageRunner requires a non-nil ProviderExecutor")
	}
	return &StageRunner{provider: provider}
}

// RunStage executes all nodes in a stage in deterministic order.
// Returns immediately on the first ambiguity or failure.
func (r *StageRunner) RunStage(
	ctx context.Context,
	executionID string,
	stage orchestrator.ExecutionStage,
	journal JournalStore,
	ambiguity AmbiguityStore,
) (StageExecutionRecord, error) {
	rec := StageExecutionRecord{
		ExecutionID: executionID,
		StageID:     stage.StageID,
		StageOrder:  stage.StageOrder,
		State:       StateExecuting,
		Outcome:     StageOutcomePending,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	// Deterministic node ordering: alphabetical by NodeID.
	nodes := make([]orchestrator.StageNode, len(stage.Nodes))
	copy(nodes, stage.Nodes)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeID < nodes[j].NodeID
	})

	for _, node := range nodes {
		nodeRec, err := r.runNode(ctx, executionID, stage.StageID, node, journal, ambiguity)
		rec.Nodes = append(rec.Nodes, nodeRec)
		if err != nil {
			rec.Outcome = StageOutcomeFailed
			rec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
			return rec, err
		}
		switch nodeRec.Outcome {
		case OutcomeAmbiguous:
			rec.Outcome = StageOutcomeAmbiguous
			rec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
			return rec, nil // stop immediately
		case OutcomeFailed:
			rec.Outcome = StageOutcomeFailed
			rec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
			return rec, nil // stop immediately
		}
	}

	rec.Outcome = StageOutcomeSuccess
	rec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	return rec, nil
}

func (r *StageRunner) runNode(
	ctx context.Context,
	executionID, stageID string,
	node orchestrator.StageNode,
	journal JournalStore,
	ambiguity AmbiguityStore,
) (NodeExecutionRecord, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	nodeRec := NodeExecutionRecord{
		ExecutionID: executionID,
		StageID:     stageID,
		NodeID:      node.NodeID,
		Action:      node.Action,
		Provider:    node.Provider,
		State:       StateExecuting,
		Outcome:     OutcomePending,
		StartedAt:   now,
	}

	// Journal entry BEFORE provider action — typed outcome "started".
	preEntry := orchestrator.NewJournalEntry(
		executionID, "", "", stageID, node.NodeID,
		node.Action, "starting node execution",
	)
	preEntry.Outcome = orchestrator.JournalOutcomeStarted
	_ = journal.Append(*preEntry)

	req := ProviderRequest{
		ExecutionID: executionID,
		StageID:     stageID,
		NodeID:      node.NodeID,
		Provider:    node.Provider,
		Action:      node.Action,
	}

	// Validate preconditions.
	validation, err := r.provider.Validate(ctx, req)
	if err != nil {
		nodeRec.Outcome = OutcomeFailed
		nodeRec.ErrorDetail = fmt.Sprintf("validate error: %v", err)
		nodeRec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		_ = journal.Append(preEntry.WithFailure(orchestrator.FailureClassRetryable, "", ""))
		return nodeRec, nil
	}
	if !validation.Valid {
		nodeRec.Outcome = OutcomeFailed
		nodeRec.ErrorDetail = "validation blocked: " + validation.BlockReason
		nodeRec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		_ = journal.Append(preEntry.WithFailure(orchestrator.FailureClassRetryable, "", ""))
		return nodeRec, nil
	}

	// Execute.
	result, err := r.provider.Execute(ctx, req)
	if err != nil {
		nodeRec.Outcome = OutcomeFailed
		nodeRec.ErrorDetail = fmt.Sprintf("execute error: %v", err)
		nodeRec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		_ = journal.Append(preEntry.WithFailure(orchestrator.FailureClassRetryable, "", ""))
		return nodeRec, nil
	}
	nodeRec.ProviderRequestID = result.RequestID

	if result.AmbiguityKind != "" {
		amb := NewAmbiguityRecord(executionID, stageID, node.NodeID, result.AmbiguityKind, result.ErrorDetail)
		_ = ambiguity.Record(amb)
		nodeRec.AmbiguityID = amb.AmbiguityID
		nodeRec.Outcome = OutcomeAmbiguous
		nodeRec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		_ = journal.Append(preEntry.WithFailure(orchestrator.FailureClassProviderAmbiguous, "", amb.AmbiguityID))
		return nodeRec, nil
	}

	// Verify outcome.
	verification, err := r.provider.Verify(ctx, req)
	if err != nil {
		amb := NewAmbiguityRecord(executionID, stageID, node.NodeID, "verify_error", err.Error())
		_ = ambiguity.Record(amb)
		nodeRec.AmbiguityID = amb.AmbiguityID
		nodeRec.Outcome = OutcomeAmbiguous
		nodeRec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		return nodeRec, nil
	}
	if !verification.Verified {
		kind := verification.AmbiguityKind
		if kind == "" {
			kind = "unverified"
		}
		amb := NewAmbiguityRecord(executionID, stageID, node.NodeID, kind, "verification did not confirm success")
		_ = ambiguity.Record(amb)
		nodeRec.AmbiguityID = amb.AmbiguityID
		nodeRec.Outcome = OutcomeAmbiguous
		nodeRec.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		return nodeRec, nil
	}

	nodeRec.Outcome = OutcomeSuccess
	nodeRec.CompletedAt = time.Now().UTC().Format(time.RFC3339)

	// Journal entry AFTER successful action — typed outcome "success".
	_ = journal.Append(preEntry.WithOutcome(orchestrator.JournalOutcomeSuccess))
	return nodeRec, nil
}
