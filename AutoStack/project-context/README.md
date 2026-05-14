# AutoStack Project Context

## Purpose

This directory contains the living memory of AutoStack's implementation state. Every session must update these files to prevent context loss.

## Structure

```
/project-context/
├── README.md                                  # This file - navigation and overview
├── current-state.md                           # What's implemented, what's broken, what's next
├── architecture/
│   ├── current-architecture.md
│   ├── architectural-review.md
│   └── control-plane-paranoia-findings.md     # Phase 1.9 — ownership, incidents, blind spots
├── providers/
│   ├── cloudrun-status.md
│   ├── provider-interface.md
│   ├── provider-limitations.md                # Phase 1.9 — honest capability inventory
│   ├── rollback-semantics.md                  # Phase 1.9 — why Rollback is refused
│   └── eventual-consistency-assumptions.md    # Phase 1.9 — provider truth lag
├── reconciler/
│   ├── reconciler-status.md
│   ├── state-model.md
│   ├── failure-classification.md
│   ├── stabilization-changes.md
│   ├── lifecycle-assumptions.md               # Phase 1.9 — runtime divergence from state model
│   ├── reconciliation-guarantees.md           # Phase 1.9 — what the loop is and isn't
│   ├── restart-behavior.md                    # Phase 1.9 — what survives a crash
│   ├── deploy-dispatch-design.md              # Phase 2.0 — CAS-claimed dispatcher design
│   └── operation-ownership.md                 # Phase 2.0 — single-ownership invariant for operations
├── known-issues/
│   ├── current-blockers.md
│   ├── dangerous-edge-cases.md                # Phase 1.9 — paranoia inventory
│   ├── correctness-limitations.md             # Phase 1.9 — truthful capability list
│   ├── deferred-operational-hardening.md      # Phase 1.9 — what's punted, why
│   ├── orphan-defense-policy.md               # Phase 2.1 — cloud hard-delete refusal
│   └── phase2.2-assessment.md                 # Phase 2.2 — end-to-end validation
├── security/
│   ├── current-security-posture.md
│   └── encryption-design.md                   # Phase 2.0 — AES-GCM, env-var key, versioned ciphertext
├── phase2.3/                                  # Phase 2.3 — control-plane integrity audit
│   ├── README.md
│   └── (16 assessment + safety-fix docs)
├── phase2.4/                                  # Phase 2.4 — lifecycle closure + retry hardening
│   ├── README.md
│   ├── lifecycle-closure-integrity-review.md
│   ├── reconciliation-convergence-assessment.md
│   ├── retry-amplification-review.md
│   ├── drift-persistence-assessment.md
│   ├── operation-retention-ttl-proposal.md
│   ├── rollback-survivability-assessment.md
│   ├── ownership-integrity-review.md
│   ├── incident-reconstruction-maturity-review.md
│   ├── operational-entropy-assessment.md
│   ├── dangerous-ambiguity-inventory.md
│   ├── phase3-readiness-assessment.md
│   ├── deferred-phase2.5-concerns.md
│   └── remaining-operational-blockers.md
├── phase2.5/                                  # Phase 2.5 — operation retention + cleanup
│   ├── README.md
│   ├── operation-cleanup-design.md
│   ├── cleanup-safety-analysis.md
│   ├── retention-policy.md
│   └── deferred-followups.md
├── phase2.6/                                  # Phase 2.6 — chaos + runtime sweep
│   ├── README.md
│   ├── chaos-scenarios-catalog.md
│   ├── runtime-sweep-design.md
│   ├── succeeded-stale-guard.md
│   └── deferred-followups.md
├── phase2.7/                                  # Phase 2.7 — forensic completeness
│   ├── README.md
│   ├── forensic-completeness-assessment.md
│   ├── release-lost-ownership-history.md
│   ├── structured-logging-proposal.md
│   └── deferred-followups.md
├── phase2.8/                                  # Phase 2.8 — drift + truthfulness maturity
│   ├── README.md
│   ├── drift-handling-maturity.md
│   ├── post-destroy-confirm-poll.md
│   ├── manual-cloud-mutation-policy.md
│   └── deferred-followups.md
├── phase2.9/                                  # Phase 2.9 — Phase 2 finalization + closure
│   ├── README.md
│   ├── lifecycle-contracts.md                 # DC-1..DC-8
│   ├── provider-contracts.md                  # P-1..P-15
│   ├── reconciliation-architecture-freeze.md  # F-1..F-9, E-1..E-4, U-1..U-3
│   ├── operational-guarantee-matrix.md        # G-1..G-19
│   ├── safe-operational-boundaries.md
│   ├── architectural-weaknesses.md            # AW-C1..C3 + AW-S1..S5
│   ├── mandatory-fixes-implementation.md      # AW-C1/C2/C3 implementation record
│   ├── production-readiness-gate.md
│   ├── trustworthiness-verdict.md             # Phase 2 verdict
│   ├── deferred-followups.md
│   ├── deferred-Phase3-concerns.md            # 19-item Phase 3 backlog
│   └── (9 supporting assessments)
├── phase3/                                    # Phase 3 — multi-provider orchestration foundations
│   ├── README.md                              # Sub-phase 3.0..3.8 sequencing
│   ├── phase3-architecture-evolution.md       # HC-1..HC-8, non-goals
│   ├── multi-provider-risk-analysis.md        # R-1..R-12
│   ├── provider-capability-matrix.md          # C-* capability framework
│   ├── provider-normalization-rules.md        # NORMALIZE/AMBIGUATE/EXPOSE rules
│   ├── ambiguity-semantics-model.md           # S-1..S-5 sources, P-1..P-6 propagation
│   ├── provider-contract-evolution.md         # Provider interface changes
│   ├── lifecycle-normalization-model.md       # N-1..N-8 mapping rules
│   └── future-ha-boundary-analysis.md         # Phase 4+ deferrals
├── decisions/
│   └── phase1-decisions.md
├── debugging/
│   └── go-module-issues.md
└── sessions/                                  # Empty placeholder
```

## Update Policy

After every major implementation step, update:
- current-state.md
- relevant subsystem file
- session handoff

## Critical Rules

1. Never rely on chat memory - write to files
2. Document blockers immediately
3. Update status after every session
4. Keep assumptions visible

## Phase Closure Status

- Phase 1.9 — closed
- Phase 2.0 / 2.1 / 2.2 — closed
- Phase 2.3 (audit + 5 fixes) — closed
- Phase 2.4 (lifecycle closure + retry hardening) — closed
- Phase 2.5 (cleanup) — closed
- Phase 2.6 (chaos + runtime sweep) — closed
- Phase 2.7 (forensic completeness) — closed
- Phase 2.8 (truthfulness maturity) — closed
- Phase 2.9 (Phase 2 finalization) — closed
- Phase 3.0 (foundation contracts) — docs landed; implementation not yet started ([phase3/README.md](phase3/README.md))
- Phase 3.1+ — not started
