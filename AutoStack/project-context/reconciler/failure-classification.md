# Failure Classification System

## Last Updated
2025-05-13

## Purpose

The failure classification system categorizes errors to make intelligent retry decisions. Not all failures should be retried - some indicate permanent issues that won't resolve with retry.

## Failure Categories

| Category | Retry Behavior | Examples |
|----------|--------------|----------|
| `transient` | YES - retry with backoff | Rate limits, temporary network issues, service unavailable |
| `permanent` | NO - never retry | Invalid credentials, malformed requests, resource not found |
| `quota` | NO - requires intervention | Quota exceeded, rate limit exceeded, billing issues |
| `auth` | NO - requires credential fix | Invalid credentials, expired tokens, permission denied |
| `timeout` | MAYBE - retry with longer timeout | Slow API response, network timeout |

## Classification Logic

### Error Pattern Matching

```go
// Transient patterns
var transientPatterns = []string{
    "timeout",
    "timed out",
    "temporary failure",
    "unavailable",
    "service temporarily unavailable",
    "connection reset",
    "network error",
}

// Permanent patterns
var permanentPatterns = []string{
    "not found",
    "invalid",
    "malformed",
    "bad request",
    "400",
    "422",
}

// Quota patterns
var quotaPatterns = []string{
    "quota",
    "limit exceeded",
    "rate limit",
    "429",
    "billing",
    "resource exhausted",
}

// Auth patterns
var authPatterns = []string{
    "unauthorized",
    "401",
    "403",
    "forbidden",
    "permission denied",
    "credentials",
    "authentication",
    "invalid token",
    "expired",
    "access denied",
}
```

### Classification Priority

1. Auth errors (highest priority) - never retry without credential fix
2. Quota errors - never retry until quota adjusted
3. Permanent errors - never retry, fix the request first
4. Transient errors - retry with exponential backoff
5. Timeout errors - retry with extended timeout

## Error Code Mapping

### Cloud Run (GCP) Error Codes

| Error | Category | Retry |
|-------|----------|-------|
| `NOT_FOUND` | permanent | No |
| `PERMISSION_DENIED` | auth | No |
| `RESOURCE_EXHAUSTED` | quota | No |
| `QUOTA_EXCEEDED` | quota | No |
| `RATE_LIMIT_EXCEEDED` | transient | Yes |
| `INTERNAL` | transient | Yes |
| `UNAVAILABLE` | transient | Yes |
| `DEADLINE_EXCEEDED` | timeout | Maybe |

### HTTP Status Code Mapping

| Status | Category | Retry |
|--------|----------|-------|
| 400 Bad Request | permanent | No |
| 401 Unauthorized | auth | No |
| 403 Forbidden | auth | No |
| 404 Not Found | permanent | No |
| 409 Conflict | permanent | No |
| 422 Unprocessable Entity | permanent | No |
| 429 Too Many Requests | quota | No |
| 500 Internal Server Error | transient | Yes |
| 502 Bad Gateway | transient | Yes |
| 503 Service Unavailable | transient | Yes |
| 504 Gateway Timeout | timeout | Maybe |

## Retry Decision Matrix

| Error Type | Default Retry | Max Retries | Backoff Multiplier |
|------------|--------------|-------------|-------------------|
| Transient | Yes | 3 | 2x |
| Timeout | Maybe | 2 | 1.5x |
| Quota | No | 0 | N/A |
| Auth | No | 0 | N/A |
| Permanent | No | 0 | N/A |

## Implementation in Reconciler

### Failure Tracking Per Target

```go
type FailureRecord struct {
    TargetID      string
    FailureCount  int
    Category      FailureCategory
    LastError     string
    LastAttempt   time.Time
}
```

### Retry Decision Flow

```
1. Provider returns error
2. Classify error into category
3. If category == permanent || category == quota || category == auth:
   - Do NOT retry
   - Update status to "error"
   - Log failure (sanitized)
   - Return
4. If category == transient || category == timeout:
   - Increment failure count for target
   - If failure count >= threshold:
     - Circuit opens, skip target
   - Else:
     - Apply backoff
     - Schedule retry
```

## Circuit Breaker Interaction

The circuit breaker tracks failures per target:

- **Closed**: Normal operation, failures increment but don't block
- **Open**: Too many failures, target skipped until backoff resets
- **Half-Open**: Testing if target is healthy again

### Failure Count Reset

Failures reset to 0 when:
- Successful `GetStatus()` call
- Successful `Deploy()` call
- Manual intervention
- Circuit breaker timeout (after max backoff)

## Logging and Observability

### Log Format

```
[FAILURE] target={targetID} category={category} message={sanitizedMessage} retry={yes/no}
```

### Metrics to Track

- `failure_total{category}` - Total failures by category
- `failure_transient` - Transient failure count
- `failure_permanent` - Permanent failure count
- `failure_auth` - Auth failure count
- `failure_quota` - Quota failure count
- `circuit_open_count` - Times circuit opened
- `retry_attempt_count` - Total retry attempts

## Future Improvements (Phase 2)

- Configurable retry policies per provider
- Adaptive retry based on success rate
- Circuit breaker per provider (not per target)
- Alerting on quota/auth failures (require intervention)