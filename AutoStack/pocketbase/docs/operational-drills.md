# AutoStack Operational Drills

Operational drills validate that operators know how to respond to known failure modes. Each drill documents the expected system behavior, the expected operator response, and what recovery paths are NOT supported.

Run these drills in a staging environment before promoting to production.

---

## Drill 1 — Contradiction Event

**Scenario:** A worker reports a running container that does not match the desired state (e.g., image tag mismatch).

**Expected system response:**
1. `DetectProviderContradiction()` raises a contradiction with `Kind=deploy-missing` or `Kind=scale-changed`
2. Contradiction appears in the operational surface (`ContradictionAlertView`)
3. No automatic remediation occurs
4. Contradiction is recorded in the evidence store (append-only)

**Expected operator response:**
1. Review contradiction detail in the UI (kind, severity, observed vs. desired state)
2. Decide: re-deploy, scale, or accept divergence
3. Issue the appropriate action (trigger execution, cancel, quarantine)
4. Contradiction is resolved when the next observation matches desired state

**Unsupported recovery paths:**
- Auto-remediation (no "fix it" button that bypasses operator decision)
- Silent contradiction suppression
- Automatic retry without operator approval

---

## Drill 2 — Replay Mismatch

**Scenario:** A replay of an execution produces a different task count or hash than the original certification.

**Expected system response:**
1. `VerifyReplayManifest()` returns `(false, reason)` with the hash mismatch
2. Replay divergence is recorded in the audit trail
3. No automatic re-replay occurs

**Expected operator response:**
1. Investigate source of divergence (missing events? clock issue? tampered store?)
2. Run `VerifyAuditChain()` to check store integrity
3. If events are missing: investigate WAL or backup state
4. If store is intact: re-run the audit with `RunProductionTruthAudit()`
5. Escalate to engineering if divergence cannot be explained

**Unsupported recovery paths:**
- Automatic divergence acceptance
- Silent re-certification on divergence
- Auto-patching of missing events

---

## Drill 3 — Corrupted Backup

**Scenario:** A backup manifest hash does not match the stored payload.

**Expected system response:**
1. `VerifyBackupManifest()` returns `(false, reason)`
2. `RestorePlatformSnapshot()` refuses restoration
3. Corruption is reported; no state mutation occurs

**Expected operator response:**
1. Identify which backup is corrupted (manifest hash vs. payload hash)
2. Retrieve an earlier valid backup from the registry
3. Run `VerifyBackupManifest()` and `VerifyPayloadIntegrity()` on the fallback backup
4. Restore from the verified fallback
5. Investigate corruption source (storage failure, transmission error, tampering)

**Unsupported recovery paths:**
- Auto-repair of corrupted backup payload
- Silent restoration from a corrupted source
- Automatic fallback to earlier backup without operator selection

---

## Drill 4 — Governance Deadlock

**Scenario:** An approval step is required but the designated approver is unavailable.

**Expected system response:**
1. Execution remains in `WaitingForApproval` state
2. Pending approval is visible in `ApprovalQueue` on the operational surface
3. No auto-approval occurs after any timeout
4. Queue is durable — persists across restarts

**Expected operator response:**
1. Identify the blocked approval step in the UI
2. Escalate to a secondary approver with the `RoleApprover` permission
3. The secondary approver reviews and decides (approve or deny)
4. Execution proceeds or is halted based on the decision

**Unsupported recovery paths:**
- Auto-escalation to a secondary approver
- Timeout-based auto-approval
- Admin override that bypasses approval evidence

---

## Drill 5 — Worker Quarantine

**Scenario:** A worker fails to send heartbeats; it is detected as potentially crashed.

**Expected system response:**
1. Heartbeat gap exceeds lease expiry threshold
2. Worker is marked quarantined
3. Tasks assigned to the quarantined worker are raised as contradictions
4. No automatic task re-assignment occurs
5. Quarantine state is visible in the operational surface

**Expected operator response:**
1. Investigate the quarantined worker (infrastructure failure? resource exhaustion?)
2. Decide: restart the worker, replace it, or re-assign tasks manually
3. If re-assigning: trigger a new execution for the affected tasks
4. Confirm the new worker sends heartbeats before declaring resolved

**Unsupported recovery paths:**
- Automatic task re-assignment from quarantined worker
- Silent quarantine override without operator action
- Auto-restart of quarantined workers

---

## Drill 6 — Archive Tamper Detection

**Scenario:** An archive entry's hash does not match its content.

**Expected system response:**
1. `VerifyArchiveIntegrity()` reports hash mismatch for the affected entry
2. Integrity report is generated (does not halt on first failure — all entries are checked)
3. No auto-repair of the tampered entry

**Expected operator response:**
1. Review `ArchiveIntegrityReport` — identify all tampered or missing entries
2. Attempt to retrieve the original from a backup (`RestorePlatformSnapshot`)
3. If no valid backup exists: record the integrity failure in the audit trail
4. Escalate to security team if tampering is suspected (not hardware failure)

**Unsupported recovery paths:**
- Automatic archive repair
- Silent acceptance of tampered entries
- Re-hashing of tampered entries to make them "pass"

---

## Drill 7 — Tenant Isolation Violation

**Scenario:** `VerifyTenantIsolation()` returns a matching isolation hash for two different tenant IDs.

**Expected system response:**
1. Isolation violation is detected and returned as `(false, reason)`
2. The request is rejected before any data access occurs
3. Violation is recorded in the audit trail as `AuditEventIsolationCheck` with `outcome=denied`

**Expected operator response:**
1. Immediately investigate the source of the violation
2. Check if two tenants were inadvertently given the same tenant ID (should be caught by `ValidateTenantID`)
3. Check if the KV store has a namespace collision (requires filesystem inspection)
4. Invalidate all active tokens for both tenant IDs until investigation completes
5. Escalate to security team

**Unsupported recovery paths:**
- Automatic tenant re-isolation
- Silent data access after a detected isolation violation
- Auto-invalidation of tokens (requires operator-triggered revocation)

---

## Drill 8 — Expired JWT Flood

**Scenario:** A client floods the auth endpoint with expired JWT tokens (DoS attempt or misconfigured client).

**Expected system response:**
1. `VerifyJWT()` returns `{Valid: false, Reason: "token expired"}` for each request
2. Rate limiter (`DefaultAuthRateLimit`: 20/min) kicks in for the tenant/IP key after threshold
3. Rate-limited requests receive `RateLimitDecision{Allowed: false}`
4. All auth failures are recorded in the audit trail as `AuditEventAuthFailure`
5. Auth failure count is incremented in `PlatformMetrics.AuthFailures`

**Expected operator response:**
1. Monitor `autostack_auth_failures_total` metric — threshold ≥10 triggers health degradation
2. Identify the source IP or tenant ID generating the flood
3. If misconfigured client: notify the tenant to rotate their tokens
4. If malicious: block at the network/load balancer level (outside AutoStack scope)
5. Review audit trail for the affected tenant after the incident

**Unsupported recovery paths:**
- Automatic IP blocking (outside AutoStack scope; belongs in network layer)
- Auto-invalidation of the flooding token (requires manual revocation)
- Distributed rate limiting across replicas (rate limiter is per-process)

---

## Common Drill Checklist

Before any drill:
- [ ] Confirm staging environment is isolated from production
- [ ] Confirm backup registry has at least one valid entry
- [ ] Confirm audit store is running and `VerifyAuditChain()` passes
- [ ] Confirm metrics snapshot baseline is captured

After any drill:
- [ ] `VerifyAuditChain()` passes on the audit store
- [ ] All contradictions raised during the drill are resolved or documented
- [ ] `RunProductionReadinessAudit()` passes all 10 gates
- [ ] No fmt.Println or credential values appear in logs

---

*Last updated: Phase PR-2. Schema version: 7.0.0.*
