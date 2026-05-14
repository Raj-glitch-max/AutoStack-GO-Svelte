# Phase 2.7 — Observability Integrity & Incident Reconstruction

## Last Updated
2026-05-14

## Goal

Make the control plane debuggable during real incidents without
grep archaeology. Phase 2.3 + Phase 2.4 covered the audit. Phase 2.7
lands a focused set of forensic improvements while deferring the
larger structured-logging refactor.

## Documents

- [Forensic-completeness assessment](forensic-completeness-assessment.md)
- [Release-lost-ownership history design](release-lost-ownership-history.md)
- [Structured logging proposal (slog)](structured-logging-proposal.md)
- [Deferred Phase 2.7 follow-ups](deferred-followups.md)

## Implementation landing in Phase 2.7

1. **History row on `[RELEASE_LOST_OWNERSHIP]`.** When the dispatcher's
   release-CAS finds 0 rows affected (sweep reclaimed the op), a
   forensic history row is now written so the operator can see the
   dispatcher's actual provider outcome.
2. **History row on CAS-race cancel.** When dispatchDeploy/destroy
   loses the CAS race, a brief history row is written.
3. **`[HEARTBEAT_FAIL_PERSISTENT]` escalation.** After 5 consecutive
   heartbeat failures for the same op, an escalated log line fires.
4. **`[CYCLE_BACKED_OFF]` debug emission.** When `reconcileWithBackoff`
   skips a cycle, a log line surfaces this so operators see backoff
   activity.

## NOT landing in Phase 2.7

- Full `log/slog` adoption (large refactor; documented as Phase 2.9 /
  Phase 3).
- Prometheus metrics emission (Phase 3).
- Per-target timeline endpoint (Phase 3 frontend).
- `operations.cycle_id` column (Phase 2.9 — bundled with multi-pod
  pod-identity work).

## Related
- [[../phase2.3/observability-integrity]]
- [[../phase2.4/incident-reconstruction-maturity-review]]
