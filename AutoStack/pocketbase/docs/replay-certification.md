# Replay Certification

**Schema Version**: 7.0.0 | **Status**: Production-ready

---

## What Replay Certification Proves

A replay certification proves that the platform's execution is **deterministic**: given the same inputs, the same sequence of scheduler events produces the same manifest hash. This is checkable offline, without re-running any execution.

It does **not** prove:
- That provider API calls produced the correct external effects
- That the execution was optimal or efficient
- That the execution completed without contradictions

---

## Replay Manifest

`pkg/replay.BuildReplayManifest(manifestID, executionID, tenantID, checkpoints, eventCount)` produces:

```go
type ReplayManifest struct {
    ManifestID   string
    ExecutionID  string
    TenantID     string
    EventCount   int
    ContentHash  string // SHA-256 of stable content fields
    ManifestHash string // SHA-256 of ContentHash + ManifestID + ExecutionID + TenantID
    CreatedAt    string // excluded from ManifestHash (content-identity)
}
```

`ContentHash` covers checkpoints only. `ManifestHash` covers all stable identity fields. `CreatedAt` is excluded — the same manifest produced at two different times produces the same hash.

---

## Verification

`replay.VerifyReplayManifest(m)` returns `(bool, string)`:

1. Recomputes `ContentHash` from checkpoints
2. Recomputes `ManifestHash` from all stable fields
3. Returns `false` with a reason if either doesn't match

This is used in the `replay-continuity` gate of operational certification.

---

## Certification Gates (pkg/opcert)

| Gate | What it checks |
|---|---|
| `replay-continuity` | `ReplayContinuityVerified=true` — manifest hash matches after restart simulation |
| `audit-durability` | Audit chain intact, entries count verified |
| `governance-continuity` | All governance decisions preserved after restart |
| `backup-recoverability` | Manifest and payload hashes both valid |
| `contradiction-survivability` | Scan completes; unresolved count visible (not blocking) |
| `restart-survivability` | Store recoverable after restart simulation |
| `revocation-integrity` | Revocations persist across store reopen |
| `deterministic-exports` | Same inputs produce same export hash |
| `staging-deployment` | Staging health endpoint responds |

All 9 gates must pass for `Certified=true`. Unresolved contradictions are visible to operators but do not block certification.

---

## Determinism Guarantee

The replay certification is valid for a **single-node, single-process** execution. It guarantees:

- Identical manifest hash for identical inputs (`-count=3` verified in CI)
- `CreatedAt` exclusion means timestamp differences never invalidate hashes
- Chain hash links each audit entry to its predecessor — tamper is detectable

It does **not** guarantee:
- Cross-replica determinism (different replicas may have diverged)
- Provider API determinism (external systems are not under platform control)
- Wall-clock ordering across process restarts (logical clock is restored, not wall clock)

---

## Operator Surface

Timeline → Replay tab shows all replay manifests for an execution with:
- Manifest ID
- Event count
- Hash verification result
- Timestamp

Forensics → Replay Manifest tab shows full hash detail for investigation.
