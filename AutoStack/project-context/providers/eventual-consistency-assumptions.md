# Provider Eventual Consistency — Assumptions and Hazards

## Last Updated
2026-05-13 (Phase 1.9 principal review)

## Premise
Cloud provider APIs are not strongly consistent. Reads can lag writes,
deletes can appear delayed, readiness can flap. Any reconciler logic that
assumes "immediate visibility" is unsafe under chaos.

## Assumptions made by current code

### Cloud Run

1. **`Deploy` → `waitForServiceReady`** assumes the first `Ready =
   SUCCEEDED` observation is stable. No "stable for N seconds" debounce.
   A revision swap during in-place update can briefly report `Ready =
   SUCCEEDED` for the OLD revision before traffic shifts.

2. **`GetStatus`** is a single-observation classifier. A flap of
   `ConfigurationsReady = RECONCILING` while `Ready = SUCCEEDED`
   previously caused `creating` to overwrite `running`. Phase 1.9 fixes
   this by:
   - giving `Ready` precedence over `ConfigurationsReady`,
   - returning `"unknown"` instead of `"pending"` when no condition matches,
   - adding a transition guard that refuses `running → pending|creating`
     on a single observation.

3. **`Destroy`** returns once `DeleteService` returns. Cloud Run may
   continue listing the service for tens of seconds afterward. The
   reconciler's next `GetStatus` may briefly observe `running` post-delete.
   No "confirm gone" loop exists today.

4. **`ValidateCredentials`** calls `ListServices` with `locations/-`
   once. A permission gap on the deployment region (the account's
   `region` field) is NOT caught at validation time — it surfaces at
   first deploy attempt.

## What we do not assume

- We do not assume revision listing is ordered. `Rollback` previously
  used `revisions[1]` from `ListRevisions`; this was wrong and is now
  refused entirely (`ErrNotImplemented`).

## Defensive measures in place (Phase 1.9)

- `GetStatus` defaults to `"unknown"`, not `"pending"`.
- Reconciler transition guard refuses single-observation regressions.
- `Destroy` predicate checks `existing.Uid != ""` (was trivially-true
  `strings.Contains(uid, "")` — masked any UID-meaning future behavior).

## Still-open hazards

- **No readiness debounce.** A flapping service will produce flapping
  status writes in the persisted record (within transition-guard
  constraints).
- **No post-delete confirmation poll.** Operators briefly see "running"
  on a service that has been deleted.
- **No region-scoped permission check.** Validation passes for accounts
  whose credentials can list services but cannot deploy in their region.

## Related
- [[lifecycle-assumptions]]
- [[rollback-semantics]]
- [[provider-limitations]]
- [[dangerous-edge-cases]]
