# Test Strategy

## Testing Goals

- protect the Kubernetes path
- verify additive cloud behavior
- catch secret leakage
- validate cost estimation trust
- test reconciliation and rollback behavior
- test user-visible event synthesis

## Test Layers

### Unit Tests
Use unit tests for:
- provider interface mapping
- cost estimation logic
- status translation
- validation helpers
- audit record creation
- WebSocket message synthesis

### Contract Tests
Use contract tests for:
- provider implementations
- API request/response shapes
- status mapping consistency
- credential validation behavior

### Integration Tests
Use integration tests for:
- PocketBase persistence
- cloud account onboarding
- deployment create/update/delete flow
- rollback flow
- registry auth behavior
- notification delivery hooks

### End-to-End Tests
Use e2e tests for:
- first deployment
- cloud account connection
- real-time status updates
- logs and metrics rendering
- destructive action confirmation flow
- rollback and redeploy

## Safety Requirements

- Tests must not depend on production credentials.
- Tests must not destroy real infrastructure unless explicitly sandboxed.
- Cloud tests should prefer mocked providers or isolated test accounts.
- Any test that touches the Kubernetes path must verify it leaves existing behavior unchanged.

## Validation Checklist

Before a change is declared safe:
- Kubernetes behavior still works
- cloud provider logic is behind the provider layer
- secrets remain redacted
- cost estimates are produced from real sources
- audit events are recorded
- destructive actions require confirmation
