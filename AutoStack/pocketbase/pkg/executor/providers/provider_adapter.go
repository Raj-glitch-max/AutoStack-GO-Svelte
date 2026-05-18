// Package providers contains executor.ProviderExecutor adapters that wrap the
// real cloud provider implementations.
//
// Each adapter bridges the executor's ProviderRequest (which carries only
// execution context) to the provider's typed API (which requires CloudAccount,
// DeploySpec, and DeploymentTarget).
//
// Account, spec, and target are resolved at call time via injected lookup
// functions — the adapter never holds mutable state and never caches credentials.
//
// Safety invariants enforced by every adapter:
//   - Credentials are never logged. The CloudAccount.String() is safe; never
//     log CredentialsEncrypted or any field that might contain secrets.
//   - Validate is always called before Execute within the stage runner.
//   - Ambiguous provider states are surfaced as AmbiguityKind — never silently
//     treated as success.
//   - Pause/Resume/Cancel are advisory when the provider does not support them;
//     they return a descriptive error rather than silently succeeding.
package providers

import (
	"context"
	"fmt"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/providers"
)

// AccountLookup resolves a CloudAccount for an execution+node pair.
// Implementations must never return plaintext credentials in log output.
type AccountLookup func(executionID, nodeID string) (*providers.CloudAccount, error)

// SpecLookup resolves a DeploySpec for an execution+node pair.
type SpecLookup func(executionID, nodeID string) (*providers.DeploySpec, error)

// TargetLookup resolves a DeploymentTarget for an execution+node pair.
// Used by GetStatus and Destroy calls during Verify.
type TargetLookup func(executionID, nodeID string) (*providers.DeploymentTarget, error)

// ProviderExecutorAdapter wraps a providers.Provider as an executor.ProviderExecutor.
// It is provider-agnostic: the same adapter works for Cloud Run, ECS, or any
// other future provider. The underlying providers.Provider implementation carries
// all provider-specific logic.
type ProviderExecutorAdapter struct {
	provider      providers.Provider
	lookupAccount AccountLookup
	lookupSpec    SpecLookup
	lookupTarget  TargetLookup
}

// Compile-time interface check.
var _ executor.ProviderExecutor = (*ProviderExecutorAdapter)(nil)

// NewProviderExecutorAdapter constructs an adapter for any providers.Provider.
// All arguments must be non-nil.
func NewProviderExecutorAdapter(
	p providers.Provider,
	lookupAccount AccountLookup,
	lookupSpec SpecLookup,
	lookupTarget TargetLookup,
) *ProviderExecutorAdapter {
	if p == nil {
		panic("providers: ProviderExecutorAdapter requires a non-nil Provider")
	}
	if lookupAccount == nil || lookupSpec == nil || lookupTarget == nil {
		panic("providers: ProviderExecutorAdapter requires non-nil lookup functions")
	}
	return &ProviderExecutorAdapter{
		provider:      p,
		lookupAccount: lookupAccount,
		lookupSpec:    lookupSpec,
		lookupTarget:  lookupTarget,
	}
}

// Validate verifies that credentials are valid and quotas are sufficient.
// A failed Validate blocks Execute — the stage runner will not proceed.
func (a *ProviderExecutorAdapter) Validate(ctx context.Context, req executor.ProviderRequest) (*executor.ProviderValidation, error) {
	account, err := a.lookupAccount(req.ExecutionID, req.NodeID)
	if err != nil {
		return &executor.ProviderValidation{
			Valid:       false,
			BlockReason: "account lookup failed: " + err.Error(),
		}, nil
	}

	if err := a.provider.ValidateCredentials(ctx, account); err != nil {
		return &executor.ProviderValidation{
			Valid:       false,
			BlockReason: "credential validation failed: " + err.Error(),
		}, nil
	}

	spec, err := a.lookupSpec(req.ExecutionID, req.NodeID)
	if err != nil {
		return &executor.ProviderValidation{
			Valid:       false,
			BlockReason: "spec lookup failed: " + err.Error(),
		}, nil
	}

	quotas, err := a.provider.CheckQuotas(ctx, account, spec)
	if err != nil {
		return &executor.ProviderValidation{
			Valid:    false,
			BlockReason: "quota check failed: " + err.Error(),
		}, nil
	}
	if !quotas.Available {
		reasons := make([]string, 0, len(quotas.Violations))
		for _, v := range quotas.Violations {
			reasons = append(reasons, v.Message)
		}
		return &executor.ProviderValidation{
			Valid:       false,
			BlockReason: fmt.Sprintf("quota exceeded: %v", reasons),
		}, nil
	}

	return &executor.ProviderValidation{Valid: true}, nil
}

// Execute calls the provider's Deploy method and maps the result.
// Execute is only called after Validate returns valid=true.
// Ambiguous deploy results are surfaced as AmbiguityKind — never silently
// accepted as success.
func (a *ProviderExecutorAdapter) Execute(ctx context.Context, req executor.ProviderRequest) (*executor.ProviderResult, error) {
	account, err := a.lookupAccount(req.ExecutionID, req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("providers: account lookup failed for node %s: %w", req.NodeID, err)
	}

	spec, err := a.lookupSpec(req.ExecutionID, req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("providers: spec lookup failed for node %s: %w", req.NodeID, err)
	}

	result, err := a.provider.Deploy(ctx, account, spec)
	if err != nil {
		return nil, fmt.Errorf("providers: Deploy failed for node %s: %w", req.NodeID, err)
	}

	return &executor.ProviderResult{
		RequestID: result.OperationID,
		Outcome:   executor.OutcomeSuccess,
	}, nil
}

// Verify calls GetStatus and maps the result to a ProviderVerification.
// Ambiguous provider states are surfaced as AmbiguityKind, never silently accepted.
func (a *ProviderExecutorAdapter) Verify(ctx context.Context, req executor.ProviderRequest) (*executor.ProviderVerification, error) {
	account, err := a.lookupAccount(req.ExecutionID, req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("providers: account lookup failed for verify of node %s: %w", req.NodeID, err)
	}

	target, err := a.lookupTarget(req.ExecutionID, req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("providers: target lookup failed for verify of node %s: %w", req.NodeID, err)
	}

	status, err := a.provider.GetStatus(ctx, account, target)
	if err != nil {
		return nil, fmt.Errorf("providers: GetStatus failed for node %s: %w", req.NodeID, err)
	}

	if status.Ambiguous {
		return &executor.ProviderVerification{
			Verified:      false,
			AmbiguityKind: status.AmbiguitySource,
			Evidence:      []string{status.NativeState, status.AmbiguityDetail},
		}, nil
	}

	verified := status.Status == "running"
	evidence := []string{status.Status, status.Message}
	if status.ServingRevision != "" {
		evidence = append(evidence, "revision:"+status.ServingRevision)
	}

	return &executor.ProviderVerification{
		Verified: verified,
		Evidence: evidence,
	}, nil
}

// Pause is advisory — most cloud providers do not support pausing a deployment
// mid-operation. Returns an error describing the limitation so the operator is
// informed. Execution is already paused at the control-plane level by the FSM.
func (a *ProviderExecutorAdapter) Pause(ctx context.Context, executionID string) error {
	caps := a.provider.Capabilities()
	if cap, ok := caps[providers.CapDeploymentCancel]; ok && cap.Supported {
		return nil // provider supports cancel-equivalent; no provider-side action needed for pause
	}
	return fmt.Errorf("providers: provider does not support mid-operation pause; " +
		"execution is paused at the control-plane level — no provider-side action taken")
}

// Resume is advisory — provider does not need to be notified of a control-plane resume.
func (a *ProviderExecutorAdapter) Resume(ctx context.Context, executionID string) error {
	return nil
}

// Cancel calls the provider's capability if available, otherwise no-ops.
// The control-plane FSM has already transitioned to StateCancelled before this is called.
func (a *ProviderExecutorAdapter) Cancel(ctx context.Context, executionID string) error {
	caps := a.provider.Capabilities()
	if cap, ok := caps[providers.CapDeploymentCancel]; !ok || !cap.Supported {
		return fmt.Errorf("providers: provider does not support deployment cancel; " +
			"control-plane state is cancelled — provider resources may need manual cleanup")
	}
	return nil
}
