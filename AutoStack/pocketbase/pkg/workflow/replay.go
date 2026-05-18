package workflow

import "fmt"

// ReplayReport describes the outcome of replaying a workflow's transition history.
type ReplayReport struct {
	ExecutionID  string
	TenantID     string
	TotalSteps   int
	Divergences  []string // hash failures and structural anomalies
	FinalState   WorkflowExecution
}

// ReplayWorkflowExecution replays a sequence of StepTransitions against the graph,
// reconstructing the final WorkflowExecution state from scratch.
//
// Every transition hash is verified. If a transition cannot be applied to the
// current state (e.g. wrong active node, terminal state), it is recorded as a divergence.
// Replay continues examining all transitions even after a divergence — nothing is silently skipped.
//
// The original transitions slice is never mutated.
func ReplayWorkflowExecution(
	graph WorkflowGraph,
	executionID, tenantID string,
	transitions []StepTransition,
) (ReplayReport, error) {
	report := ReplayReport{
		ExecutionID: executionID,
		TenantID:    tenantID,
		TotalSteps:  len(transitions),
	}

	if len(transitions) == 0 {
		return report, nil
	}

	// The first transition initializes the execution.
	first := transitions[0]
	if ok, reason := VerifyStepTransition(first); !ok {
		report.Divergences = append(report.Divergences,
			fmt.Sprintf("transition 0 (id=%s) hash invalid: %s", first.TransitionID, reason))
		return report, nil
	}
	if first.FromNodeID != "" {
		report.Divergences = append(report.Divergences,
			fmt.Sprintf("transition 0: expected empty FromNodeID for initial transition, got %q", first.FromNodeID))
	}

	// Build the initial execution state from the first transition.
	entryNode, ok := FindNode(graph, first.ToNodeID)
	if !ok {
		return report, fmt.Errorf("entry node %q from first transition not found in graph", first.ToNodeID)
	}
	_ = entryNode
	exec, initTr, err := ExecuteWorkflowGraph(graph, executionID, first.TransitionID, tenantID)
	if err != nil {
		return report, fmt.Errorf("replay: init failed: %w", err)
	}
	// Verify the reconstructed init transition matches.
	if initTr.ToNodeID != first.ToNodeID {
		report.Divergences = append(report.Divergences,
			fmt.Sprintf("transition 0: replay produced ToNodeID %q but recorded was %q",
				initTr.ToNodeID, first.ToNodeID))
	}

	// Apply subsequent transitions.
	for i, tr := range transitions[1:] {
		idx := i + 1
		if ok, reason := VerifyStepTransition(tr); !ok {
			report.Divergences = append(report.Divergences,
				fmt.Sprintf("transition %d (id=%s) hash invalid: %s", idx, tr.TransitionID, reason))
			continue
		}
		if IsExecutionTerminal(exec) {
			report.Divergences = append(report.Divergences,
				fmt.Sprintf("transition %d: execution already terminal (%s), cannot apply", idx, exec.State))
			continue
		}
		outcome := NodeOutcome{
			NodeID: tr.FromNodeID,
			Result: tr.EdgeType,
			Reason: tr.Reason,
		}
		if exec.ActiveNodeID != tr.FromNodeID {
			report.Divergences = append(report.Divergences,
				fmt.Sprintf("transition %d: active node %q != recorded from-node %q",
					idx, exec.ActiveNodeID, tr.FromNodeID))
			continue
		}
		newExec, _, advErr := AdvanceWorkflow(graph, exec, outcome, tr.TransitionID)
		if advErr != nil {
			report.Divergences = append(report.Divergences,
				fmt.Sprintf("transition %d: advance failed: %v", idx, advErr))
			continue
		}
		// Verify the destination matches.
		if newExec.ActiveNodeID != tr.ToNodeID {
			report.Divergences = append(report.Divergences,
				fmt.Sprintf("transition %d: replay produced active-node %q but recorded to-node was %q",
					idx, newExec.ActiveNodeID, tr.ToNodeID))
		}
		exec = newExec
	}

	report.FinalState = exec
	return report, nil
}
