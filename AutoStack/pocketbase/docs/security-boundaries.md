# Security Boundaries

**Schema Version**: 7.0.0 | **Status**: Production-ready

---

## What AutoStack Secures

| Boundary | Mechanism |
|---|---|
| API authentication | JWT (HMAC-SHA256, `golang-jwt/jwt/v4`) + PocketBase auth |
| Authorization | Default-deny RBAC, 5 roles, 20+ permissions |
| Secret storage | AES-256-GCM encryption via `pkg/secrets` |
| Credential logging | Explicitly prohibited at all log levels |
| Audit trail tamper | Hash-chain; any modification detectable |
| Token revocation | SQLite-persisted revocation list, survives restart |
| Rate limiting | In-process token bucket, 5-minute sliding window |
| Tenant isolation | All store reads scoped by `tenantID` |

---

## Authentication

JWTs are issued with:
- `tenantID` claim for tenant binding
- `role` claim for RBAC
- `exp` claim for expiration
- HMAC-SHA256 signature

`VerifyJWT` checks signature, expiration, and tenant binding. It does **not** check revocation by default — callers must use `VerifyJWTWithRevocation` for revocation-aware verification.

API tokens use `hmac.Equal()` for constant-time comparison.

---

## What Is Never Logged

- JWT secrets
- Encryption keys
- Cloud provider credentials
- Secret values (only `SecretRef` is logged, never the value)
- Audit entry metadata (metadata may contain credential-adjacent context)

This is enforced by code review convention and the `pkg/secrets.SecretRef.String()` method which never exposes the underlying value.

---

## Credential Storage

Cloud provider credentials are stored via `pkg/secrets.SecretStore`:
- AES-256-GCM encryption
- Key derived from `AUTOSTACK_ENCRYPTION_KEY` env var
- Key fingerprint logged on startup (not the key itself)
- If key is missing or malformed: cloud account writes fail loudly, not silently

Credentials are never:
- Logged at any log level
- Stored in plaintext
- Transmitted in API responses

---

## Tenant Isolation

Every store read includes a `tenantID` filter:

```go
// Correct: scoped to tenant
store.EntriesForTenant(tenantID)

// Wrong: returns all tenants (never use outside admin paths)
store.AllEntries()
```

`ExportTenantAudit` is explicitly scoped. The forensics surface never cross-joins tenants.

---

## Revocation

`PersistentRevocationStore` maintains a `revoked_tokens` table in SQLite.

- `Revoke(tokenID, tenantID, reason)` is idempotent (INSERT OR IGNORE)
- `IsRevoked(tokenID)` is O(1) in-memory lookup (after initial load)
- Revocation survives store reopen — revoked tokens stay revoked

`PerformKeyRotation(rotationID, tenantID, oldTokenIDs, rl)` revokes all listed token IDs and records a rotation event. The rotation record never stores the new secret — only the token IDs that were revoked.

---

## Rate Limiting

`RateLimiter` (pkg/security) uses a sliding window token bucket:

```go
type RateLimitPolicy struct {
    MaxRequests int
    Window      time.Duration
}
```

- Thread-safe via `sync.Mutex`
- In-process only — not distributed across replicas
- Per-key: each `Check(key)` call is scoped to a string key (typically `tenantID` or `clientIP`)

**Limitation**: rate limits are not shared across replicas. A distributed rate limiter requires Redis or equivalent — not yet implemented.

---

## What AutoStack Does Not Provide

- mTLS between internal components
- HSM-backed key storage
- Secrets rotation without operator intervention
- Distributed rate limiting (single-process only)
- SSO/SAML (planned, not yet built)
- Penetration test certification (external audit not performed)

These gaps are documented honestly. Do not infer security guarantees beyond what is explicitly stated here.
