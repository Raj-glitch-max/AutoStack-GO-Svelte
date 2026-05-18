package providers

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/providers"
)

// Static compile-time interface check.
var _ executor.ProviderExecutor = (*ProviderExecutorAdapter)(nil)

func noopLookups() (AccountLookup, SpecLookup, TargetLookup) {
	return func(_, _ string) (*providers.CloudAccount, error) { return &providers.CloudAccount{}, nil },
		func(_, _ string) (*providers.DeploySpec, error) { return &providers.DeploySpec{}, nil },
		func(_, _ string) (*providers.DeploymentTarget, error) { return &providers.DeploymentTarget{}, nil }
}

func TestNewProviderExecutorAdapter_NilProvider_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("nil Provider must panic")
		}
	}()
	la, ls, lt := noopLookups()
	NewProviderExecutorAdapter(nil, la, ls, lt)
}

func TestNewProviderExecutorAdapter_NilLookups_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("nil lookup functions must panic")
		}
	}()
	NewProviderExecutorAdapter(&stubProvider{}, nil, nil, nil)
}

func TestProviderExecutorAdapter_Validate_FailsOnAccountLookupError(t *testing.T) {
	la := func(_, _ string) (*providers.CloudAccount, error) {
		return nil, fmt.Errorf("no account found")
	}
	_, ls, lt := noopLookups()
	adapter := NewProviderExecutorAdapter(&stubProvider{}, la, ls, lt)

	result, err := adapter.Validate(context.Background(), executor.ProviderRequest{
		ExecutionID: "exec-1", NodeID: "svc-a",
	})
	if err != nil {
		t.Fatalf("Validate must not return error on lookup failure: %v", err)
	}
	if result.Valid {
		t.Fatal("Validate must return valid=false when account lookup fails")
	}
	if result.BlockReason == "" {
		t.Fatal("BlockReason must be set on validation failure")
	}
}

func TestProviderExecutorAdapter_Validate_SucceedsWithValidStub(t *testing.T) {
	la, ls, lt := noopLookups()
	adapter := NewProviderExecutorAdapter(&stubProvider{}, la, ls, lt)

	result, err := adapter.Validate(context.Background(), executor.ProviderRequest{
		ExecutionID: "exec-1", NodeID: "svc-a",
	})
	if err != nil {
		t.Fatalf("Validate must not error with stub provider: %v", err)
	}
	if !result.Valid {
		t.Fatalf("Validate must return valid=true with stub provider: %s", result.BlockReason)
	}
}

func TestProviderExecutorAdapter_Resume_NoOp(t *testing.T) {
	la, ls, lt := noopLookups()
	adapter := NewProviderExecutorAdapter(&stubProvider{}, la, ls, lt)
	if err := adapter.Resume(context.Background(), "exec-1"); err != nil {
		t.Fatalf("Resume must not error: %v", err)
	}
}

// ─── stub provider ────────────────────────────────────────────────────────────

type stubProvider struct{}

func (s *stubProvider) Capabilities() providers.CapabilitySet {
	caps := providers.CapabilitySet{}
	for _, k := range providers.AllCapabilityKeys {
		caps[k] = providers.Capability{Supported: true, Semantic: "stub"}
	}
	return caps
}
func (s *stubProvider) ValidateCredentials(_ context.Context, _ *providers.CloudAccount) error {
	return nil
}
func (s *stubProvider) Deploy(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploySpec) (*providers.DeployResult, error) {
	return &providers.DeployResult{OperationID: "op-stub"}, nil
}
func (s *stubProvider) GetStatus(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploymentTarget) (*providers.TargetStatus, error) {
	return &providers.TargetStatus{Status: "running"}, nil
}
func (s *stubProvider) GetOperation(_ context.Context, _ *providers.CloudAccount, _ string) (*providers.Operation, error) {
	return &providers.Operation{Status: providers.OperationStatusSucceeded}, nil
}
func (s *stubProvider) Rollback(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploymentTarget, _ string) (*providers.DeployResult, error) {
	return &providers.DeployResult{}, nil
}
func (s *stubProvider) GetMetrics(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploymentTarget) (*providers.TargetMetrics, error) {
	return &providers.TargetMetrics{}, nil
}
func (s *stubProvider) StreamLogs(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploymentTarget, _ io.Writer) error {
	return nil
}
func (s *stubProvider) EstimateCost(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploySpec) (*providers.CostEstimate, error) {
	return &providers.CostEstimate{}, nil
}
func (s *stubProvider) GetActualCost(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploymentTarget, _, _ time.Time) (*providers.ActualCost, error) {
	return &providers.ActualCost{}, nil
}
func (s *stubProvider) Destroy(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploymentTarget) error {
	return nil
}
func (s *stubProvider) ListRegions(_ context.Context, _ *providers.CloudAccount) ([]string, error) {
	return nil, nil
}
func (s *stubProvider) CheckQuotas(_ context.Context, _ *providers.CloudAccount, _ *providers.DeploySpec) (*providers.QuotaCheck, error) {
	return &providers.QuotaCheck{Available: true}, nil
}
