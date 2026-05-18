# Forensic Model

**Schema Version**: 7.0.0 | **Status**: Production-ready

---

## Purpose

The forensic model defines how operators investigate past execution events, contradiction evidence, and certification records. All forensic operations are **read-only** — no forensic surface mutates any operational state.

---

## Forensic Evidence Types

| Type | Location | Tamper-Detection |
|---|---|---|
| Audit trail | `PersistentAuditStore` | Hash-chain (entry + chain hash) |
| Replay manifests | In-memory + store | `ManifestHash` recomputed on verify |
| Governance decisions | `GovernanceSnapshot` | Snapshot hash |
| Contradiction records | `VerificationSnapshot` | Per-contradiction evidence fields |
| Backup manifests | `BackupRegistry` | `ManifestHash` (content-identity) |
| Operational certifications | `OperationalReadinessCertification` | `CertHash` (CertifiedAt excluded) |

---

## Hash-Chained Audit Trail

Each audit entry has two hashes:

```
EntryHash  = SHA-256(entry_id + event_type + tenant_id + ... stable fields)
ChainHash  = SHA-256(prev.EntryHash + "|" + this.EntryHash)
```

`OccurredAt` is excluded from `EntryHash` (content-identity). This means:
- The same entry recorded at two times produces the same `EntryHash`
- The chain is still intact across clock skew
- Tamper is detectable because modifying any entry invalidates all subsequent `ChainHash` values

`VerifyAuditChain(store)` checks all three properties:
1. Each `EntryHash` recomputes correctly
2. Each `ChainHash` links to its predecessor
3. `OccurredAt` is monotonically non-decreasing (temporal sanity)

---

## Contradiction Evidence

A contradiction is surfaced when a provider observation conflicts with the expected state. Evidence includes:

- `ContradictionID` — unique identifier
- `Description` — human-readable description of the conflict
- `DetectedAt` — when the contradiction was detected
- `EvidenceFields` — provider-reported state vs. expected state

Contradictions are **never auto-resolved**. They remain visible until an operator explicitly acknowledges or resolves them via the governance override path.

Unresolved contradictions do **not** block operational certification — they are visible to operators as a known, auditable state.

---

## Forensic Investigation Surface

Platform → Forensics provides:

| Tab | What it shows |
|---|---|
| Snapshot | Point-in-time forensic snapshot with invariant violations |
| Replay Manifest | Full hash inspection (manifest hash, content hash, event count) |
| Certification | Gate-by-gate certification evidence with cert hash |
| Contradictions | Contradiction evidence with provider-reported field data |
| Backup | Backup manifest hash verification and payload integrity |

All data is fetched via `GET /api/v1/platform/forensics/:id` — read-only, no mutations.

---

## Export Safety

`ExportTenantAudit(store, tenantID)` produces an export that:
- Excludes `Metadata` fields (which may contain credential-adjacent context)
- Is scoped to a single tenant (no cross-tenant data)
- Preserves `EntryHash` and `ChainHash` for offline verification
- Can be verified offline without access to the original store

---

## What Forensics Cannot Tell You

- Whether a provider API call succeeded at the provider's end (external side-effect)
- Whether a governance decision was ethically correct (only that it was recorded)
- Whether a contradiction was caused by a bug or by real infrastructure drift
- Root cause analysis for complex multi-provider sequences (manual investigation required)

These gaps are honest — they reflect the boundary of what append-only evidence can prove.
