# Credential Encryption — Phase 2.0 Design

## Last Updated
2026-05-14

## Scope (and explicit non-scope)

**What Phase 2.0 ships:**
- AES-256-GCM symmetric encryption.
- Key sourced from `AUTOSTACK_ENCRYPTION_KEY` (32 raw bytes, base64).
- Versioned ciphertext format `enc:v1:<base64(nonce(12) ‖ ciphertext)>`.
- Legacy plaintext rows lazily re-encrypted on next write (validate /
  re-save).
- Refusal-to-start visible in logs if the key is missing or malformed.

**What Phase 2.0 explicitly does NOT ship** (deferred — see
`[[deferred-operational-hardening]]`):
- KMS / HSM integration.
- Per-tenant key derivation.
- Key rotation.
- At-rest encryption of the SQLite file itself.
- Encrypted backups.

The bar: "no field labeled `*_encrypted` should ever contain plaintext
after Phase 2.0 ships." Anything beyond that is Phase 3.

## Threat model

Defended against:
- Read of `cloud_accounts.credentials_encrypted` from PocketBase admin
  UI by an unprivileged user (rule already requires auth) — value is
  ciphertext.
- SQLite file backup theft — value is ciphertext.
- Accidental dump of the column in logs — value is ciphertext.

NOT defended against:
- Compromise of `AUTOSTACK_ENCRYPTION_KEY` itself.
- Memory dump during a Provider.Deploy call (plaintext is in process
  memory during the API call — acceptable for Phase 2.0).
- Operator with PocketBase admin password (admin role can decrypt by
  invoking the validate endpoint).

## Implementation

`pkg/secrets/secrets.go` exposes:

- `Encrypt(plaintext) (string, error)` — returns `enc:v1:...` or empty.
- `Decrypt(stored) (string, error)` — handles v1 ciphertext and legacy
  plaintext; refuses to fall through on v1 corruption.
- `IsEncrypted(stored) bool` — for "should I re-encrypt on write?"
  decisions.
- `EnsureConfigured() error` — call at process boot.
- `KeyFingerprint() string` — SHA-256(key)[0..4] hex, for logs.

### Write boundary

`HandleCloudAccountCreate` calls `secrets.Encrypt` before saving. If
encryption fails (key missing/malformed), the handler returns a 500
rather than writing plaintext.

### Read boundary

Every code path that calls `Provider.{ValidateCredentials,Deploy,
GetStatus,Destroy,ListRegions,EstimateCost}` first calls
`secrets.Decrypt` on the stored value. The plaintext lives only in the
local variable passed to the provider; it is never re-written to disk
unencrypted.

The reconciler (`pkg/reconciler/cloud.go`) decrypts at the top of
`reconcileOne`. A decryption failure for a v1-prefixed value is treated
as an auth-class failure: the target → `error`, no retry storm. Logged
as `[CRED_DECRYPT_FAIL]`.

### Lazy migration

`HandleCloudAccountValidate` checks `secrets.IsEncrypted` on the stored
value. If it is legacy plaintext, the value is re-saved as v1
ciphertext on this same validation pass. After all accounts have been
validated once post-Phase-2.0, all records are encrypted.

A one-shot batch re-encryption migration is not provided. The lazy
approach lets operators upgrade in place without downtime.

## Key management

- The key MUST be 32 raw bytes encoded as base64 (44 chars).
- A wrong-key scenario (operator rotated the key without re-encrypting
  data) is detected: `Decrypt` returns `ErrCiphertextCorrupt`. The
  reconciler treats this as a per-target auth failure.
- A missing-key scenario is detected at boot via `EnsureConfigured`,
  logged as `[ENCRYPTION_NOT_CONFIGURED]`. Cloud-account writes/reads
  then fail until the operator fixes it.
- `KeyFingerprint()` is logged at startup as `[ENCRYPTION_READY]
  key_fp=...`. Operators can compare fingerprints across pods to
  catch silent key drift.

## What this design assumes

1. The Go process has read access to the environment variable.
   (Standard for containerized deploys with secrets mounted as env vars
   or files-into-env.)
2. `AUTOSTACK_ENCRYPTION_KEY` is stable across process lifetimes.
   Changing it without re-encrypting data breaks decryption — detected,
   not silently fixed.
3. The PocketBase admin role is treated as a trusted role for Phase 2.0.

## Operator guidance

1. Generate a key:
   `openssl rand -base64 32`
2. Set `AUTOSTACK_ENCRYPTION_KEY=<that-output>` in the environment.
3. Restart the process. Verify the log line `[ENCRYPTION_READY]
   key_fp=<8 hex chars>`.
4. Existing cloud_accounts will continue to work as plaintext until
   their next validate call, at which point they will re-encrypt
   automatically.

## Related
- [[deferred-operational-hardening]]
- [[dangerous-edge-cases]]
- [[correctness-limitations]]
