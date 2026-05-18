# AutoStack v7.0.0 — Release Notes

**Release date**: 2026-05-17 | **Schema version**: 7.0.0

---

## What This Release Is

AutoStack v7.0.0 is the first production-ready release of the orchestration platform. It is not a feature-complete cloud management suite — it is a **deterministic, replay-certified orchestration platform** with operator-first operational control.

This release does not claim to be everything. It claims to be honest about what it is.

---

## Platform Claim

> A deterministic orchestration platform with replay-certified execution, append-only operational evidence, governance-enforced automation, durable forensic auditability, and operator-first operational control.

---

## What's New in v7.0.0

### Phase PR-1: Security Hardening & Production Readiness
- `pkg/security`: Default-deny RBAC (5 roles, 20+ permissions), JWT + revocation, durable audit trail, rate limiter, secret store
- `pkg/chaos`: Deterministic fault injection engine (10 fault types)
- `pkg/metrics`: Platform-wide metric counters, gauges, histograms
- `pkg/backup`: Backup manifests with content-identity hashing, 10-gate readiness audit
- `pkg/loadtest`: Load scenario runner with reproducible worker/iteration patterns
- `pkg/prodcert`: 10-gate production certification (tamper-evident)
- CI: `.github/workflows/platform-hardening.yml` — 6 CI jobs including race detector and benchmark smoke

### Phase PR-2: Durability Hardening & Operational Realism
- `pkg/security/audit_store.go`: Durable SQLite WAL audit trail (hash-chained, restart-safe)
- `pkg/security/revocation.go`: JWT revocation + key rotation (persistent, idempotent)
- `tests/integration/crash/`: 7 crash survivability tests
- `tests/integration/storage/`: 9 storage durability tests
- `benchmarks/production/`: 13 production benchmarks with measured baseline results
- `pkg/opcert`: 9-gate operational readiness certification (tamper-evident)
- `docs/distributed-boundaries.md`, `docs/operational-drills.md`
- `deploy/staging/`, `release/rc/validate.sh`

### Phase PX-1: Platform Maturity & Operator Experience
- **Operator Timeline UI**: Execution, contradiction, replay, governance, and audit-chain timeline tabs
- **Forensic Investigation Surface**: Snapshot, replay manifest, certification, contradiction evidence, backup verification
- **Governance Operations Center**: Policy decisions, overrides, approval hierarchies
- **Operational Drill Mode**: 8 simulated drills, clearly marked SIMULATION ONLY
- **Installer**: `scripts/install/install.sh` — target <15 minutes to operational
- **Demo Environment**: `demo/` with `make demo-up`, `make demo-reset`
- **Documentation**: 6 hardened docs (runtime-architecture, replay-certification, governance-model, operational-survivability, forensic-model, security-boundaries)
- **Final Product Certification**: 11-gate `PlatformProductReadinessCertification` (tamper-evident)

---

## Certification Summary

| Certification | Gates | Status |
|---|---|---|
| Production Readiness (pkg/backup) | 10 | All pass |
| Production Certification (pkg/prodcert) | 10 | All pass |
| Operational Certification (pkg/opcert) | 9 | All pass |
| Platform Product Certification | 11 | All pass |

Total tests: **215+** across 50+ packages. Zero failures.

---

## Known Limitations (Documented)

These are not bugs — they are honest scope boundaries:

1. **Rate limiting is in-process only** — not distributed across replicas
2. **Restart survivability tests use in-process simulation** — real kill-9 requires external harness
3. **Cross-replica audit chain continuity not guaranteed** — single-node authoritative
4. **Worker quarantine state does not persist across restart** — re-quarantine required by operator
5. **Approval hierarchies do not expire** — approvals remain pending indefinitely until resolved
6. **Demo environment uses synthetic data** — no real cloud provider connections
7. **Distributed consensus not implemented** — Raft/Paxos not in scope

---

## Breaking Changes

None. v7.0.0 is the first tagged release. Previous work was pre-release development.

---

## Upgrade Path

From pre-release: No migration required. Data schema version `7.0.0` is the initial production schema.

---

## What Is NOT in This Release

- SSO/SAML authentication
- Multi-replica HA deployment
- Distributed rate limiting
- Cross-replica revocation propagation
- External security audit / penetration test certification
- Auto-scaling (planned)
- Cost estimation UI (backend implemented, UI pending)

---

## Contributors

Built by Raj Patil with Claude Code (Anthropic).
