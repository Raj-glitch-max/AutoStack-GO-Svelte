# Known Issues - Current Blockers

## Last Updated
2025-05-13

## CRITICAL BLOCKERS

### 1. Build Verification Needed
**Severity**: High
**Status**: RESOLVED
**Issue**: Code now compiles successfully
**Resolution**: Fixed Go module imports for Cloud Run SDK

### 2. Go Module Resolution Failure
**Severity**: Critical
**Status**: RESOLVED
**Issue**: `cloud.google.com/go/run` import was using wrong path
**Resolution**: Changed to `cloud.google.com/go/run/apiv2` import pattern

## HIGH PRIORITY ISSUES

### 3. Cost Estimation Is Placeholder
**Severity**: High (per ADR-010)
**Status**: Known, documented as placeholder
**Issue**: Uses hardcoded static values instead of live pricing API
**Impact**: Estimates may be significantly wrong
**Resolution**: Call GCP Cloud Billing API when available

### 4. Metrics Return Zeros
**Severity**: Medium
**Status**: Intentional placeholder
**Issue**: `GetMetrics()` returns `{CPUPercent: 0, MemoryMB: 0, RequestsCount: 0}`
**Impact**: No real metrics for Cloud Run deployments
**Resolution**: Implement Cloud Monitoring API integration

### 5. GetActualCost Not Implemented
**Severity**: Medium
**Status**: Returns error
**Issue**: `GetActualCost()` returns `fmt.Errorf("actual cost retrieval not yet implemented")`
**Impact**: Cannot show actual vs estimated cost
**Resolution**: Implement Cloud Billing API integration

## MEDIUM PRIORITY ISSUES (from KNOWN_ISSUES.md)

### 6. No Circuit Breaker (ISSUE-018)
**Severity**: Medium
**Status**: Known limitation
**Issue**: Reconciler continues retrying on API failures without circuit breaker
**Impact**: Wasted API calls during provider outages
**Resolution**: Implement per-provider failure tracking and circuit breaker

### 7. No Orphan Cleanup (ISSUE-019)
**Severity**: Medium
**Status**: Known limitation
**Issue**: If cloud delete fails, resources may be orphaned
**Impact**: Cost accumulation for deleted deployments
**Resolution**: Add periodic orphan scan job

### 8. Reconciliation Has No Backoff
**Severity**: Medium
**Status**: Not implemented
**Issue**: Errors retry immediately on next 30s cycle
**Impact**: Rapid retry during sustained failures
**Resolution**: Add exponential backoff or fixed delay on errors

### 9. No Distributed Lock
**Severity**: Medium (for HA deployments)
**Status**: Not implemented
**Issue**: Multiple backend instances would race during reconciliation
**Impact**: Duplicate API calls, potential corruption
**Resolution**: Implement PocketBase-based distributed lock

### 10. Cloud Run SDK Not Verified
**Severity**: High
**Status**: Unverified
**Issue**: API correctness not confirmed due to build failure
**Impact**: Code may not compile or may have wrong API usage
**Resolution**: Fix build, then test with real credentials

## UNVERIFIED ASSUMPTIONS

| Assumption | Risk | Status |
|-----------|------|--------|
| Cloud Run SDK import path is correct | High | Unverified (build fails) |
| Service account JSON structure is correct | Medium | Assumed |
| GCP region list is complete | Low | May be outdated |
| Service name validation is correct | Low | Implemented but untested |
| Container port handling is correct | Low | Implemented but untested |

## Security Issues (Documented)

### 11. Credential Encryption Key Management (ISSUE-002)
**Severity**: Critical
**Status**: Open
**Issue**: Encryption key from env var, not KMS
**Impact**: If key leaks, all credentials exposed
**Resolution**: Integrate with cloud KMS (Phase 3)

### 12. Error Sanitization Implemented
**Severity**: Medium
**Status**: Implemented in reconciler
**Solution**: `sanitizeError()` function redacts sensitive patterns
**Verification**: Untested

## Issues NOT Addressed (Scope Boundaries)

- Multi-tenancy (ISSUE-003) - Phase 2 work
- Audit logging (ISSUE-004) - Phase 2 work
- SOC2 compliance (ISSUE-005) - Phase 4
- VPC management (ISSUE-015) - Phase 2
- Blueprint versioning (ISSUE-010) - Phase 2

## Recommendations

1. **Immediate**: Fix Go module resolution to enable build
2. **Short-term**: Verify Cloud Run SDK API correctness
3. **Medium-term**: Implement circuit breaker and orphan cleanup
4. **Long-term**: Address security (KMS) and multi-tenancy