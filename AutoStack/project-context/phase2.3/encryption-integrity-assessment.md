# Encryption Integrity Assessment — Phase 2.3

## Last Updated
2026-05-14

## Bar

The encryption subsystem must **fail closed**, never open. A missing
key, a wrong key, a tampered ciphertext, a partial-migration mix of
plaintext and ciphertext — none of these may cause AutoStack to
silently fall back to operating on plaintext credentials.

## What's in place today (Phase 2.0/2.1)

- AES-256-GCM with `AUTOSTACK_ENCRYPTION_KEY` (32 bytes, base64).
- Versioned ciphertext format `enc:v1:<b64(nonce||ct)>`.
- `EnsureConfigured()` returns error if key missing or malformed.
- `[ENCRYPTION_NOT_CONFIGURED]` logged at boot if EnsureConfigured
  fails.
- `[ENCRYPTION_READY] key_fp=<hex>` logged at boot when configured.
- `secrets.Decrypt` returns `ErrCiphertextCorrupt` on key mismatch /
  tampered ciphertext.
- Reconciler's `[CRED_DECRYPT_FAIL]` path treats decryption failure as
  auth-class (no retry; target → error).
- Legacy plaintext (no `enc:v1:` prefix) lazily re-encrypts on next
  write via the controller boundary.
- `CloudAccount.String()`/`GoString()` redact credentials so they
  cannot leak via `%v`/`%+v`/structured logging.

## Audit-level questions

### EI-1: Does the process refuse to start without a key?

**Today:** No. `main.go` calls `EnsureConfigured()` and logs
`[ENCRYPTION_NOT_CONFIGURED]` but **continues running**.

```go
if err := secrets.EnsureConfigured(); err != nil {
    log.Printf("[ENCRYPTION_NOT_CONFIGURED] %v — ...", err)
} else {
    log.Printf("[ENCRYPTION_READY] key_fp=%s", secrets.KeyFingerprint())
}
```

**Impact:** Cloud-account writes fail in `HandleCloudAccountCreate`
because `secrets.Encrypt` returns ErrKeyMissing → 500 error to the
client. Cloud-account reads fail in the reconciler because
`secrets.Decrypt` returns ErrKeyMissing → target → error. **But the
process keeps running**, and the Kubernetes path keeps working (no
crypto needed). This is intentional — we don't want a missing key to
take down the Kubernetes deployment system.

**Verdict:** Acceptable. Cloud features fail closed; Kubernetes
features continue. The trade-off is documented.

**Alternative considered:** Refuse to start. Rejected because it
couples cloud-feature health to Kubernetes-feature operability.

### EI-2: What if the key is rotated without re-encrypting data?

**Today:** Existing ciphertext was encrypted under key K1. New process
runs with key K2. `Decrypt` returns `ErrCiphertextCorrupt` for any
ciphertext. The reconciler's `[CRED_DECRYPT_FAIL]` path treats this as
auth-class failure: target → error. No retry.

**Impact:** All cloud deployments halt. Operator must either
(a) restore K1, or (b) re-create cloud_accounts with new credentials
under K2.

**Verdict:** Honest failure. No silent fallback. ✓

**Operator UX gap:** No "key rotation supported" path. Phase 3 KMS
work would add envelope encryption with rotatable DEK.

### EI-3: What about partial migration?

**Today:** Some cloud_accounts may have plaintext
`credentials_encrypted`, others may have ciphertext.

`Decrypt`:
- Plaintext (no `enc:v1:` prefix) → returns as-is. **Plaintext fallback
  for legacy rows.**
- `enc:v1:` prefix → decrypts.

**Severity of "as-is plaintext fallback":** Acceptable IF lazy
migration is reliable.

**Lazy migration:** `HandleCloudAccountValidate` re-encrypts any row
where `secrets.IsEncrypted` returns false. After all accounts have been
validated once post-Phase-2.0, all rows are encrypted.

**Hazard:** If an operator NEVER hits the validate endpoint, the
legacy plaintext lingers. Reads still work (`Decrypt` passes plaintext
through). But the ciphertext-at-rest promise is unmet.

**Phase 2.3 mitigation:** Document that operators MUST validate all
cloud_accounts once after Phase 2.0 deployment. Or run a one-shot
migration script. **No code change.**

**Phase 2.5 work:** A one-shot migration that re-encrypts every
plaintext row on a flag, OR remove the plaintext fallback entirely
(forcing decrypt of plaintext to fail) — making the system fail closed
on plaintext-after-migration.

### EI-4: Memory exposure during Provider call

**Today:** Decrypted credentials are passed into `Provider.Deploy`/
`Provider.GetStatus` as `account.CredentialsEncrypted` (named
misleadingly — it's plaintext at this point). The Cloud Run provider
unmarshals to `CloudRunCredentials` struct and calls
`run.NewServicesClient(..., option.WithCredentialsJSON(...))`.

**Plaintext lifetime:** From the reconciler's `secrets.Decrypt` call
through the provider call return. After return, the local variables
go out of scope; Go GC eventually collects.

**Memory-dump risk:** A core dump during this window exposes
plaintext.

**Verdict:** Acceptable for Phase 2.0/2.3. Documented as Phase 3 KMS
work (in-memory short-lived decryption with auto-zeroing).

### EI-5: Serialization exposure

**Today:** `CloudAccount.String()`/`GoString()` redact the credentials
field. Any `log.Printf("%v", account)` or `fmt.Sprintf("%+v", account)`
in code that imports the providers package would invoke this and
redact.

**However:** Code that imports a different struct (e.g.,
`reconciler/cloud.go`'s local `row map[string]interface{}` from
SQL select) could accidentally `log.Printf("%v", row)` and leak the
`credentials_encrypted` field's CIPHERTEXT (not plaintext, since the
SQL select returns the ciphertext-on-disk value).

**Verdict:** Ciphertext exposure in logs is acceptable (the whole
point of ciphertext is "safe to log"). Plaintext exposure is blocked
because plaintext only exists inside the provider call's local scope.

**Phase 2.5 work:** Add a `Decrypt`-returning-`Credential` type with
a `MustNotLog` marker; static analysis catches accidental logging.

### EI-6: Replay safety with encrypted credentials

**Today:** On restart, the reconciler reads `credentials_encrypted`
from disk (ciphertext), calls `Decrypt`, passes plaintext to provider.
No state survives across restart that contains plaintext. ✓

**Verdict:** Replay-safe.

### EI-7: Rotation assumptions

**Today:** Key is sourced from env var once per process. `cachedAEAD`
in secrets package is set once, cached for the process lifetime. A
key rotation requires a process restart.

**Verdict:** Documented limitation. Phase 3 KMS work would add live
rotation support.

### EI-8: Fail-closed on Encrypt

**Today:** `Encrypt(plaintext)` returns `(string, error)`.
`HandleCloudAccountCreate`:
- Calls `secrets.Encrypt`. If err, returns HTTP 500.
- If empty plaintext, returns empty string — does not encrypt nothing.

**Verdict:** Cannot silently write plaintext. ✓

### EI-9: Fail-closed on Decrypt

**Today:** `Decrypt(stored)` returns:
- `""` if stored is empty.
- The plaintext if stored is non-empty no-prefix (LEGACY FALLBACK).
- The decrypted plaintext if stored is `enc:v1:...` and key matches.
- `ErrCiphertextCorrupt` if stored is `enc:v1:...` and key mismatch or
  tampered.
- `ErrKeyMissing` if no key is configured.

**Verdict:** Fails closed on tamper. **Falls open on legacy plaintext
detection.**

The legacy fallback is the only honest concern. Documented in EI-3.

### EI-10: Boot-time key fingerprint logging

**Today:** `[ENCRYPTION_READY] key_fp=<hex>` is logged at boot. The
fingerprint is `sha256(key)[0..4]` hex (4 bytes = 8 hex chars).
Operators across pods can compare to detect key drift.

**Verdict:** ✓ Sufficient.

## Phase 2.3 implementation in this area

None. The encryption subsystem is correctly fail-closed for everything
except the documented legacy-plaintext fallback, which is mitigated by
lazy migration.

## Deferred

- EI-3 mitigation upgrade (one-shot migration + plaintext-rejection
  flag) — Phase 2.5.
- EI-7 (live key rotation) — Phase 3 KMS.
- EI-5 (static analysis for accidental logging) — Phase 2.5.

## Related
- [[../security/encryption-design]]
- [[../known-issues/dangerous-edge-cases]] (mitigation section)
