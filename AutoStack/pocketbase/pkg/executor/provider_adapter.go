package executor

import "context"

// ProviderExecutor defines the contract between the execution runtime and
// cloud providers. No direct SDK calls are allowed in the executor — all
// provider interaction must go through this interface.
//
// Each method must:
//   - Return explicit ambiguity (never hide partial state)
//   - Expose provider request IDs for forensic tracing
//   - Never log secrets or credentials
//   - Be callable independently for verification and cancellation
type ProviderExecutor interface {
	// Validate checks provider-side preconditions before execution begins.
	// A failed validation blocks execution without calling Execute.
	Validate(ctx context.Context, req ProviderRequest) (*ProviderValidation, error)

	// Execute submits the intended action to the provider.
	// Returns AmbiguityKind non-empty when the provider state cannot be confirmed.
	Execute(ctx context.Context, req ProviderRequest) (*ProviderResult, error)

	// Verify checks provider-side state after execution to confirm the outcome.
	// Returns Verified=false and AmbiguityKind non-empty when state is uncertain.
	Verify(ctx context.Context, req ProviderRequest) (*ProviderVerification, error)

	// Pause suspends provider-side operation if the provider supports it.
	// Returns nil if the provider does not support pause (not an error).
	Pause(ctx context.Context, executionID string) error

	// Resume continues a previously paused operation.
	Resume(ctx context.Context, executionID string) error

	// Cancel terminates a provider operation cleanly.
	Cancel(ctx context.Context, executionID string) error
}

// NoOpProviderExecutor is a safe placeholder that rejects all provider calls.
// Used as the default when no real provider is configured, and in tests
// that verify executor behaviour without provider interaction.
type NoOpProviderExecutor struct{}

func (n *NoOpProviderExecutor) Validate(_ context.Context, _ ProviderRequest) (*ProviderValidation, error) {
	return &ProviderValidation{Valid: false, BlockReason: "no provider configured"}, nil
}

func (n *NoOpProviderExecutor) Execute(_ context.Context, _ ProviderRequest) (*ProviderResult, error) {
	return &ProviderResult{Outcome: OutcomeFailed, ErrorDetail: "no provider configured"}, nil
}

func (n *NoOpProviderExecutor) Verify(_ context.Context, _ ProviderRequest) (*ProviderVerification, error) {
	return &ProviderVerification{Verified: false, AmbiguityKind: "no provider configured"}, nil
}

func (n *NoOpProviderExecutor) Pause(_ context.Context, _ string) error  { return nil }
func (n *NoOpProviderExecutor) Resume(_ context.Context, _ string) error { return nil }
func (n *NoOpProviderExecutor) Cancel(_ context.Context, _ string) error { return nil }

// SucceedingProviderExecutor is a test double that always validates, executes,
// and verifies successfully. Safe for use in tests that need a provider that works.
type SucceedingProviderExecutor struct{}

func (s *SucceedingProviderExecutor) Validate(_ context.Context, _ ProviderRequest) (*ProviderValidation, error) {
	return &ProviderValidation{Valid: true}, nil
}

func (s *SucceedingProviderExecutor) Execute(_ context.Context, req ProviderRequest) (*ProviderResult, error) {
	return &ProviderResult{
		RequestID: "req:" + req.ExecutionID + ":" + req.NodeID,
		Outcome:   OutcomeSuccess,
	}, nil
}

func (s *SucceedingProviderExecutor) Verify(_ context.Context, _ ProviderRequest) (*ProviderVerification, error) {
	return &ProviderVerification{Verified: true}, nil
}

func (s *SucceedingProviderExecutor) Pause(_ context.Context, _ string) error  { return nil }
func (s *SucceedingProviderExecutor) Resume(_ context.Context, _ string) error { return nil }
func (s *SucceedingProviderExecutor) Cancel(_ context.Context, _ string) error { return nil }

// AmbiguousProviderExecutor is a test double that always returns ambiguity from Execute.
type AmbiguousProviderExecutor struct{}

func (a *AmbiguousProviderExecutor) Validate(_ context.Context, _ ProviderRequest) (*ProviderValidation, error) {
	return &ProviderValidation{Valid: true}, nil
}

func (a *AmbiguousProviderExecutor) Execute(_ context.Context, req ProviderRequest) (*ProviderResult, error) {
	return &ProviderResult{
		RequestID:     "req:" + req.NodeID,
		Outcome:       OutcomeAmbiguous,
		AmbiguityKind: "timeout",
		ErrorDetail:   "provider timed out",
	}, nil
}

func (a *AmbiguousProviderExecutor) Verify(_ context.Context, _ ProviderRequest) (*ProviderVerification, error) {
	return &ProviderVerification{Verified: false, AmbiguityKind: "timeout"}, nil
}

func (a *AmbiguousProviderExecutor) Pause(_ context.Context, _ string) error  { return nil }
func (a *AmbiguousProviderExecutor) Resume(_ context.Context, _ string) error { return nil }
func (a *AmbiguousProviderExecutor) Cancel(_ context.Context, _ string) error { return nil }
