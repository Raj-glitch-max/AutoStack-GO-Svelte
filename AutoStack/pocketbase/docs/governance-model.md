# Governance Model

**Schema Version**: 7.0.0 | **Status**: Production-ready

---

## Principle: Operator Authority Is Never Bypassed

AutoStack's governance model has one non-negotiable rule:

> **No automated path produces an outcome that an operator did not authorize.**

This means:
- No auto-approval when approvers are unavailable
- No silent escalation that unblocks execution without a logged decision
- No policy "fallback to allow" under load or timeout
- No execution that skips a `require-approval` gate because the governance service is slow

If governance is blocked, **execution is blocked**. Operators unblock it.

---

## RBAC Model

Five roles with default-deny permissions:

| Role | Can Do |
|---|---|
| `viewer` | Read executions, governance history, metrics |
| `operator` | Approve/deny actions, trigger rollbacks, manage workers |
| `approver` | Approve actions requiring approval; cannot initiate |
| `auditor` | Read-only access to audit trail, forensic data |
| `admin` | Full platform access including credential management |

`CheckPermission(role, perm)` is default-deny: any permission not explicitly listed in `rolePermissions` returns false.

---

## Policy Effects

| Effect | Meaning |
|---|---|
| `allow` | Action proceeds immediately |
| `deny` | Action is blocked permanently (can only be overridden by admin) |
| `require-approval` | One approver with `permission:approve` must authorize |
| `require-multi-approval` | Multiple approvers from a defined hierarchy must authorize |

---

## Approval Hierarchies

An approval hierarchy defines a sequence of approver steps. Each step must be satisfied in order. The hierarchy is `AllSatisfied=true` only when all steps are completed.

```go
type ApprovalHierarchy struct {
    HierarchyID  string
    ExecutionID  string
    PolicyID     string
    Rationale    string
    Steps        []ApprovalStep
    AllSatisfied bool
    CreatedAt    string
    Hash         string // tamper-evident
}
```

If an approver at step N is unavailable, **step N is not auto-satisfied**. The hierarchy remains incomplete and the execution remains blocked.

---

## Overrides

An override is an explicit governance record that an operator has taken manual control. It requires:
1. An `ActorID` with sufficient permissions
2. A `Reason` (mandatory)
3. An `OverriddenAt` timestamp

Overrides are **not retroactive** — they do not validate past decisions. They record that an operator knowingly bypassed the normal governance path and accepted responsibility.

All overrides are visible in:
- Platform → Governance → Overrides tab
- Timeline → Governance tab
- Audit trail (audit event recorded)

---

## Audit Chain

Every governance decision, approval, denial, and override is recorded in the hash-chained audit trail. Each entry's `ChainHash = SHA-256(prev.EntryHash + "|" + entry.EntryHash)`.

Tamper detection: `VerifyAuditChain(store)` checks all entry hashes and chain hashes. Any modification to any entry invalidates all subsequent chain hashes — making tamper detectable without needing a central trust anchor.

---

## What Governance Does Not Cover

- Provider API idempotency (external systems may process a request twice)
- Network partition behavior (blocked execution may hold locks)
- Approval expiry (approvals do not expire — this is a known gap, documented in KNOWN_ISSUES)
- Cross-replica governance consensus (single-node authoritative)
