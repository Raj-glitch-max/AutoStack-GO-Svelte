# Operational Survivability

**Schema Version**: 7.0.0 | **Status**: Production-ready

---

## What "Operational Survivability" Means

A platform is operationally survivable if, after a restart, crash, or store reopen:

1. All evidence is intact and verifiable
2. No committed decisions were lost
3. Operators can reconstruct what happened
4. The platform does not silently resume from an inconsistent state

AutoStack provides all four.

---

## Survivability Gates (pkg/opcert)

| Gate | Recovery Mechanism |
|---|---|
| Replay continuity | Manifest hash re-verified from store on start |
| Audit durability | Hash-chained entries survive store reopen |
| Governance continuity | Governance snapshots persisted to SQLite |
| Backup recoverability | Manifest + payload hash verified before restore |
| Contradiction survivability | Scan completes after restart; results re-surfaced |
| Restart survivability | `DurableAuditRecorder.Restore()` + `RecoverPersistentRuntime()` |
| Revocation integrity | `INSERT OR IGNORE` ensures idempotent revocation |
| Deterministic exports | Same inputs → same hash; verified post-restart |
| Staging deployment | Health endpoint verified before claiming production-ready |

---

## Store Recovery Sequence

On process start:
```
1. Open SQLiteKVStore (WAL mode)
2. Open PersistentAuditStore (WAL mode)
3. DurableAuditRecorder.Restore() — loads all entries, rebuilds in-memory trail
4. Open PersistentRevocationStore — all revocations available immediately
5. Replay manifest hashes verified from store
6. Governance snapshots loaded (no auto-replay of decisions)
7. Scheduler queue rebuilt from persisted task state
8. Workers start as healthy (quarantine state does not persist across restart — operator must re-quarantine if needed)
```

---

## What Survives a Restart

| Item | Survives? | How |
|---|---|---|
| Audit trail entries | Yes | SQLite WAL, hash-chained |
| JWT revocations | Yes | SQLite `INSERT OR IGNORE` |
| Governance decisions | Yes | Snapshot to SQLite |
| Replay manifests | Yes | Stored content hash |
| Task state | Yes | SQLite KV store |
| Worker quarantine | No | Re-quarantine required by operator |
| In-memory metrics | No | Restart resets counters |
| Rate limiter state | No | In-process only |

---

## Crash Semantics

AutoStack uses in-process crash simulation (no real `kill -9` test without an external harness). The simulation covers:

- Aborted writes: duplicate write rejected by UNIQUE constraint — no partial state
- Replay hash mismatch: detected on first `VerifyReplayManifest` call after restart
- Governance deadlock: blocked execution stays blocked — no auto-resume
- Archive tamper: `VerifyAuditChain` detects at first verification call

---

## Known Gaps

- Worker quarantine state does not persist — operator must re-quarantine after restart
- Scheduler queue in-process state rebuilds from SQLite, but priority order may differ
- Cross-replica survivability is not implemented (single-node authoritative)
- WAL durability requires `PRAGMA synchronous = FULL` — fsync on every commit; this is safe but slow on spinning disks
