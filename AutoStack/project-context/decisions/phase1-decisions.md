# Phase 1 Decisions

## Last Updated
2025-05-13

## Decision Framework

This file documents architectural decisions made during Phase 1 implementation. Per DECISIONS.md convention, decisions include context, decision, consequences, and alternatives.

---

## DECISION-P1-001: Provider Interface Location

**Date**: 2025-05-13
**Status**: Accepted
**Context**: Need to define where cloud provider abstractions live in the codebase
**Decision**: Create `/pkg/providers/` package with interface definition and implementations as sub-packages
**Consequences**:
- Provider implementations isolated in their own packages
- Single entry point for provider access via registry pattern
- Easy to add new providers without modifying core code
**Alternatives Considered**:
- Put providers in pkg/cloud/ (rejected: /pkg/providers/ follows Go conventions)
- Put interface in pkg/cloud/ and implementations in pkg/cloud/providers/ (rejected: deeper nesting)
- Use plugin system with dynamic loading (rejected: over-engineered for current needs)

---

## DECISION-P1-002: Cloud Account Credentials Format

**Date**: 2025-05-13
**Status**: Accepted
**Context**: How to store and transmit cloud provider credentials
**Decision**: Store credentials as encrypted JSON blob in PocketBase, decrypt in-memory at use time
**Consequences**:
- Single field can contain provider-specific credential structure
- Encryption is at rest; credentials in memory during API calls
- Credential format varies by provider (GCP uses service account JSON, AWS uses access key + secret)
**Alternatives Considered**:
- Separate fields per credential type (rejected: rigid, doesn't handle provider variations)
- Store encrypted reference to external secret manager (rejected: Phase 3 work)

---

## DECISION-P1-003: Reconciler Polling Interval

**Date**: 2025-05-13
**Status**: Accepted
**Context**: How often should cloud deployment status be checked
**Decision**: Default 30-second polling interval, configurable via Config struct
**Consequences**:
- 30 seconds is balance between drift detection speed and API call volume
- Configurable allows users to trade responsiveness for cost
- Per ADR-022, 30 seconds is the standard
**Alternatives Considered**:
- 10 seconds (rejected: 3x API calls, marginal latency benefit)
- 60 seconds (rejected: slow drift detection)
- Adaptive (faster when deploying) (rejected: complexity, marginal value)

---

## DECISION-P1-004: Error Sanitization Strategy

**Date**: 2025-05-13
**Status**: Accepted
**Context**: How to prevent credential leakage in logs when cloud API calls fail
**Decision**: Implement sanitizeError() function that redacts sensitive patterns before logging
**Consequences**:
- Pattern-based redaction is simple and fast
- May miss some edge cases
- Does not require re-architecting logging system
**Alternatives Considered**:
- Structured logging with field-level redaction (rejected: more complex)
- Never log errors (rejected: operational debugging impossible)
- Use separate audit log for errors (rejected: Phase 2 work)

---

## DECISION-P1-005: Service Name Validation

**Date**: 2025-05-13
**Status**: Accepted
**Context**: Cloud Run requires specific service name format (lowercase, max 63 chars)
**Decision**: Truncate and lowercase at deployment time, format as `autostack-{truncated-id}`
**Consequences**:
- Guarantees valid service names
- May cause collisions if truncation identical (mitigated by using rollout ID)
- User can still specify readable names in target_config
**Alternatives Considered**:
- Fail deployment if name invalid (rejected: poor UX)
- Generate random suffix (rejected: harder to debug)

---

## DECISION-P1-006: Cost Estimation Approach

**Date**: 2025-05-13
**Status**: Deferred (Placeholder)
**Context**: Cost estimates must use live pricing APIs per ADR-010
**Decision**: Implemented placeholder with hardcoded values, documented as non-production
**Consequences**:
- Code compiles and runs
- Estimates may be significantly wrong
- User sees warning that estimate is placeholder
**Alternatives Considered**:
- Call GCP Cloud Billing API immediately (rejected: complexity, need to handle API errors)
- Return error for cost estimation (rejected: blocks user workflow)

---

## DECISION-P1-007: PocketBase/dbx Import Path

**Date**: 2025-05-13
**Status**: Accepted
**Context**: Original import was `github.com/pocketbase/pocketbase/dbx` which failed
**Decision**: Changed to `github.com/pocketbase/dbx` (separate module)
**Consequences**:
- Import works for reconciler
- May need to verify other files use correct path
**Alternatives Considered**:
- Keep original path and investigate (rejected: build fails)
- Use PocketBase SDK methods instead (rejected: requires refactoring)

---

## Related ADRs from DECISIONS.md

- ADR-005: Provider interface pattern (followed)
- ADR-006: PocketBase as single source of truth (preserved)
- ADR-007: Cloud reconciler pattern (followed)
- ADR-008: Cloud Run as first provider (followed)
- ADR-010: Cost estimates as ranges (partial - documented as placeholder)

---

## Outstanding Questions

1. Should we use go mod vendor to avoid dependency issues?
2. Should we implement cost estimation via external service (Infracost)?
3. Should circuit breaker use per-provider or per-target granularity?