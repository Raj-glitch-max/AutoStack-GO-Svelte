# AI_OPERATING_RULES.md — Rules of Engagement for AI Agents

> **READ THIS BEFORE ANY ACTION.** Together with `CLAUDE.md` and `KUBERNETES_EXISTING_SYSTEM.md`, this file defines exactly how an AI agent must behave inside this repository. Violations break production. Violations cost money. Violations destroy trust. There is no version of "I forgot" that is acceptable in infrastructure code.

---

## Core Purpose

This file exists because AI agents pattern-match. They generate confident-looking output that may be wrong. They forget context across long sessions. They invent function names that don't exist, schema fields that don't exist, environment variables that don't exist. They refactor things they were not asked to refactor. They "improve" things that were working.

AutoStack is infrastructure. Infrastructure code that confidently fails causes outages. Outages cost real money for real users. The rules in this file exist to make that pattern impossible inside this repository.

---

## The Five Inviolable Rules

### Rule 1: Read Before You Write

Before any code change, you must have read:
- `CLAUDE.md` — every time, every session, before anything
- `KUBERNETES_EXISTING_SYSTEM.md` — every time you might touch the Kubernetes codepath
- The specific section of `ARCHITECTURE.md` for the layer you are working in
- `DATA_MODEL.md` — if you are touching PocketBase collections in any way
- `DECISIONS.md` — to verify the change you are about to make has not already been explicitly rejected
- `KNOWN_ISSUES.md` — to verify you are not fixing something that was deliberately deferred

If you have not read these, you do not have enough context to make a safe change. Stop. Read first. Then act.

### Rule 2: Never Refactor Without Explicit Instruction

You will be tempted to "clean up" code as you work. You will see patterns that look inefficient. You will see naming conventions that look inconsistent. You will see file structures that look improvable.

**Do not touch them.**

If a function works and is not part of your assigned task, you do not modify it. If you find code you believe should be refactored, you record it in `KNOWN_ISSUES.md` with the label `refactor-candidate` and continue with the actual task.

The Kubernetes system is the most extreme version of this rule: you do not refactor it. You do not improve it. You do not modernize it. It works. Working infrastructure is more valuable than elegant infrastructure.

### Rule 3: No Cloud-Specific Logic Outside `/pkg/providers/`

The Provider interface in `/pkg/providers/provider.go` is the only place cloud-specific logic lives. There is no `if provider == "aws"` anywhere else in the codebase. There is no AWS SDK import anywhere except in `/pkg/providers/aws/`. There is no GCP-specific configuration parsing anywhere except in `/pkg/providers/gcp/`.

If you find yourself wanting to add a cloud-specific check in a generic path, you are doing it wrong. Step back. The right answer is always to add a method to the Provider interface and implement it in each provider package.

### Rule 4: Credentials and Secrets Are Sacred

There is no log statement, at any log level, that contains a credential, secret, API key, password, token, or session identifier in plaintext. Ever. There is no exception. There is no debug mode that allows this. There is no "just for development" pattern that does this.

You do not write credentials to stdout. You do not write them to stderr. You do not write them to a file outside of the designated encrypted secret store. You do not include them in error messages that get returned to the user.

If a credential needs to be logged, it is the redacted form: first three characters plus `***` plus last two characters. If a secret needs to be referenced, it is by ID, never by value. If an error includes a credential, the error is sanitized before it leaves the function that holds the credential.

### Rule 5: Stop and Ask Before Destructive Operations

A destructive operation is:
- Any deletion of cloud resources
- Any deletion of Kubernetes resources
- Any deletion of PocketBase records (even with cascade)
- Any modification of the audit log
- Any change to encrypted credential fields
- Any modification of the operator RBAC
- Any change to the CRD schema
- Any production deployment of AutoStack itself

For any of these, you stop and confirm with the user before proceeding. Confirmation is explicit. "I should delete this resource, is that correct?" is not enough. "I am about to delete resource X with ID Y in account Z, this is irreversible, please confirm" is the required form.

---

## What You Are Allowed To Do Without Asking

Additive changes inside the scope of an assigned task:
- Add new files in the correct package location per `CLAUDE.md` folder conventions
- Add new functions, types, or methods that follow established patterns
- Add new tests for existing or new code
- Add new PocketBase collections for new features (with migration files)
- Add new documentation files or sections
- Add comments explaining non-obvious code
- Fix typos in comments or strings
- Update versions in `DECISIONS.md` when you add a dependency

These changes still require:
- Reading the relevant existing code first
- Following established conventions in this repository
- Not breaking any existing test
- Not modifying any file outside the immediate scope of the task

---

## What You Are Not Allowed To Do Without Asking

| Operation | Why It Is Forbidden Without Approval |
|---|---|
| Modify any file in `/pkg/kubernetes/operator/` | The operator is complete and working |
| Modify any file in `/pkg/kubernetes/controllers/` | Reconciliation logic is locked |
| Modify the CRD definition in `/api/v1alpha1/` | Schema changes require migration plan |
| Remove or rename a PocketBase field | Breaks existing records |
| Change a PocketBase collection name | Breaks existing API contracts |
| Modify a database migration file after it has been applied | Migrations are immutable |
| Change authentication or session logic | Security-critical, requires review |
| Modify the audit log writing code | The audit log must be append-only |
| Change any RBAC policy in deployment manifests | Production permissions, requires review |
| Add a new external dependency | Documented in `DECISIONS.md` first |
| Modify CI/CD configuration | Operational, requires explicit task |
| Change Helm chart values defaults | Deployment behavior change, requires review |
| Touch any code in `/pkg/secrets/` without a security task | Secret management is locked |
| Modify the WebSocket message schemas | Frontend contract, requires coordination |
| Change cost calculation formulas | Affects every user, requires review |
| Modify the reconciliation interval | Affects API rate limits and costs |

---

## How To Handle Ambiguity

When you encounter ambiguity, the wrong response is to guess. The right response is to state the ambiguity clearly and propose options.

### The Required Format for Ambiguity Resolution

```
I need to clarify before proceeding.

WHAT I KNOW:
- Specific fact 1 from documentation
- Specific fact 2 from existing code
- Specific fact 3 from the user's instruction

WHAT IS AMBIGUOUS:
- Specific question that needs an answer

OPTIONS:
A) [Concrete option with stated trade-offs]
B) [Concrete option with stated trade-offs]
C) [Concrete option with stated trade-offs]

MY RECOMMENDATION:
[Which option I would choose and why]

I will not proceed until you confirm.
```

Never silently assume. Never proceed with "I think this is what you meant." If you find yourself starting a code change with "I'll assume...", stop. Ask first.

---

## How To Handle Errors During Work

When you encounter an error during a task — a build failure, a test failure, an unexpected behavior — the wrong response is to "fix" it by changing the surrounding code until it works. The right response is to understand the error first.

### Required Error Investigation Sequence

1. Read the full error message. Not the summary. The full output.
2. Identify the exact file, line, and function where the error originates.
3. Read the code at that location to understand what it is doing.
4. Determine if the error is caused by your change or by a pre-existing condition.
5. If pre-existing, log it in `KNOWN_ISSUES.md` and continue with the task.
6. If caused by your change, understand why before modifying anything else.
7. Make the minimal change required to fix the error.
8. Verify the fix did not introduce a new problem in a related area.

You do not work around errors by commenting code out. You do not work around errors by silencing tests. You do not work around errors by adding broad exception handlers. You understand the error or you stop.

---

## How To Handle Tasks Larger Than You Can Hold

Some tasks are larger than one session of focused attention. Some tasks require coordinated changes across multiple files, multiple packages, multiple layers. For these, the right pattern is decomposition.

### Decomposition Sequence for Large Tasks

1. Read the user's full intent. Restate it back in your own words. Confirm.
2. Identify the layers that must change (frontend, API, backend, provider, schema).
3. Identify dependencies between changes. What must happen before what?
4. Propose a sequence of small commits or pull requests, each with one purpose.
5. Confirm the sequence with the user before starting.
6. Execute the sequence one step at a time.
7. After each step, confirm with the user that the step is correct before moving on.

This is slower than the "do everything at once" approach. It is also the only approach that produces reliable infrastructure software.

---

## How To Handle Multi-File Coordination

A single feature often touches multiple files. The discipline for multi-file work is:

1. List every file you intend to touch before touching any of them.
2. Justify each touch: "I am modifying this file because..."
3. Identify the order in which files must change to avoid breaking the build at any intermediate state.
4. Execute the changes in that order.
5. Run the build after each file to verify nothing intermediate is broken.

If you find yourself modifying a file you did not list, stop. Why did you need to modify it? Does this indicate a missed dependency? Should the scope be reconsidered?

---

## How To Handle the Kubernetes System

You do not touch the Kubernetes system unless the assigned task explicitly says so. This includes:

- Any file in `/pkg/kubernetes/`
- The CRD definitions
- The operator manifest
- The operator RBAC
- The Kubernetes deployment migration files in PocketBase
- The Kubernetes-related WebSocket watchers
- The Kubernetes cluster configuration UI in the frontend

If you are working on a cloud feature and you find yourself needing to modify a Kubernetes file, stop. The likely answer is that you have made the cloud feature too coupled to the Kubernetes path, or that you have misunderstood the Provider interface.

The Provider interface is the bridge. Cloud features extend the Provider interface and implement it in `/pkg/providers/<cloud>/`. Cloud features do not modify the Kubernetes path.

---

## How To Handle Migrations

PocketBase migrations are immutable after they are applied. The rules:

1. A migration that has been merged to the main branch has been applied somewhere. Treat it as applied.
2. To change a previous migration, you write a new migration that reverses or modifies it. You do not edit the old migration.
3. Every migration must include both `up` and `down` operations.
4. Every migration must be tested against existing data, not just an empty database.
5. Migrations that add nullable fields are safe. Migrations that add non-nullable fields without defaults are forbidden.
6. Migrations that rename fields are forbidden in this codebase. Add a new field, copy the data in code, deprecate the old field, drop the old field in a later migration.
7. Migrations that drop fields require a deprecation period of at least one release.

If a migration is needed and it does not fit these rules, stop and discuss with the user before writing it.

---

## How To Handle External Dependencies

A new external dependency (Go module, npm package, system package) is a long-term commitment. The rules:

1. Before adding a dependency, search for whether the same capability already exists in the project.
2. Before adding a dependency, evaluate its maintenance status. Last commit date, open issues, license.
3. Before adding a dependency, evaluate its alternatives. Document why this one was chosen.
4. Record the decision in `DECISIONS.md` with: package name, version pinned, purpose, alternatives considered, who decided, when.
5. Add the dependency in the minimum scope (e.g., to one package, not to a shared module unless needed).
6. Verify the dependency does not duplicate functionality already in the project.

Forbidden dependencies (do not add without explicit security review):
- Any package that does its own HTTP server
- Any package that does its own crypto primitives outside the Go standard library
- Any package that handles credentials and is not on the approved list
- Any package with a known security advisory
- Any package not actively maintained (no commits in 18 months)

---

## How To Handle the AI Features Themselves

AutoStack uses AI for specific features: incident explanation, right-sizing recommendations, Docker Compose conversion. These are user-facing AI features. The rules for building them:

1. The AI provider is configurable per user. You do not hardcode Anthropic or OpenAI as the only option.
2. The user provides their own API key. AutoStack does not pay for the AI calls.
3. AI responses are always validated. The AI returns structured JSON, which is validated against a schema before any action is taken.
4. AI suggestions are never auto-applied. The user always sees the suggestion and decides whether to apply it.
5. AI features must work without AI. If the AI provider is unavailable, the feature degrades gracefully. The user can still operate AutoStack without AI.
6. AI prompts are versioned. When you change a prompt, you bump the prompt version. Old prompts remain accessible for debugging and comparison.
7. AI rate limits are enforced server-side. The user cannot accidentally rack up a large AI bill through a runaway loop.
8. AI inputs are sanitized. User input is never injected directly into a prompt; it goes through a templating layer that prevents prompt injection.

The AI features are differentiators, but they are not the foundation. The foundation is the working deployment system. The AI features sit on top.

---

## How To Handle the Cost Estimation Code

Cost estimation is one of the most sensitive areas of the codebase because it sets user expectations and trust. The rules:

1. Never hardcode a price. Every price comes from the cloud provider's pricing API or a cached fetch of it.
2. Every cost estimate is a range, not a point. The range expresses uncertainty about traffic, data transfer, and storage growth.
3. Every cost estimate states its assumptions explicitly. "Estimated $X to $Y per month, assuming [list of assumptions]."
4. Cost estimates are recalculated at least daily for active deployments because cloud prices change.
5. The displayed currency is always USD unless the user has explicitly selected a different currency in settings.
6. Cost estimates never include cloud account credits or reserved instance discounts unless the user has configured those.
7. The difference between "estimated cost" and "actual cost from cloud billing" must be visible to the user. They are different numbers and must not be conflated.
8. Cost anomaly detection runs in the background. If a deployment's actual cost in the last 24 hours is more than 2x its 7-day average with no configuration change, an alert fires.

---

## How To Handle Documentation Updates

Documentation evolves with the code. The rules:

1. When you change a function's signature, you update its documentation.
2. When you add a new feature, you update the relevant section of `ARCHITECTURE.md`.
3. When you make a decision, you record it in `DECISIONS.md`.
4. When you discover a problem you are not fixing now, you record it in `KNOWN_ISSUES.md`.
5. When you add a new external dependency, you document why in `DECISIONS.md`.
6. When you add a new environment variable, you add it to the environment matrix (see `DEPLOYMENT_MODEL.md`).
7. When you change a contract between layers, you update `SYSTEM_BOUNDARIES.md`.
8. When you change an API endpoint, you update `API_CONTRACTS.md`.

Documentation is part of the work. A task is not complete until the relevant documentation is updated.

---

## How To Handle "Quick Fixes"

There is no such thing as a quick fix in infrastructure. Every "quick fix" is a permanent change to a long-running system. The rules:

1. There is no commit message that begins with "quick fix" or "small change" or "minor".
2. Every change has the same review standard regardless of size.
3. A one-line change can break the entire system. Treat it accordingly.
4. The phrase "this should just work" is a warning sign. Verify before claiming.
5. Hotfixes are still proper fixes. They get tested. They get documented. They get reverted via the same pipeline if they fail.

The temptation to "just push it and see" does not exist in this codebase. Verify before pushing. Always.

---

## How To Handle Production Incidents

If you are asked to help respond to a production incident:

1. The first priority is stopping the bleeding. Restore service before investigating root cause.
2. Do not modify code in the heat of an incident unless you are certain of the fix.
3. The default response is to roll back. Rollback is always safer than forward-fix.
4. Read the runbook for the relevant component (see `/docs/runbooks/`).
5. Do not delete logs, events, or evidence during an incident. They are needed for post-incident review.
6. Communicate frequently. Status messages every 10 minutes minimum during an active incident.
7. After the incident, conduct a blameless post-mortem. Document the timeline, the root cause, the contributing factors, and the action items.
8. The post-mortem is added to `/docs/incidents/`. It is searchable. Future incidents will reference it.

---

## How To Handle User Data

User data is sacred. The rules:

1. You do not query user data unless it is required for the task.
2. You do not display user data in logs.
3. You do not include user data in error messages that leave the system.
4. You do not export user data to external services without explicit user consent.
5. You do not retain user data longer than the documented retention period.
6. You do not share user data across tenant boundaries. Multi-tenancy isolation is absolute.
7. You comply with the data residency requirements for each region. Data created in EU stays in EU.

If you are not sure whether something counts as user data, treat it as user data.

---

## How To Handle Multi-Tenant Concerns

Even in single-tenant mode today, AutoStack is being built for multi-tenancy. The rules:

1. Every database query for user data includes a user/workspace filter. There is no "get all rollouts" query without a tenant filter.
2. Every API endpoint validates the calling user has access to the requested resource. There is no implicit trust.
3. Every cloud account is owned by a workspace. Cross-workspace access is forbidden.
4. Every WebSocket connection is scoped to a user. A user cannot subscribe to another user's events.
5. Every cache key includes a tenant identifier. There is no shared cache that could leak data between tenants.
6. Every background job processes only the tenants it is scoped to. There are no "process all tenants" jobs.

When you are unsure whether a piece of code is multi-tenant safe, ask. The default behavior should be "deny across tenant boundaries."

---

## How To Handle the Frontend

The SvelteKit frontend has its own rules:

1. The frontend is a thin layer. Business logic lives in the backend API.
2. The frontend does not call cloud APIs directly. Every cloud operation goes through the backend.
3. The frontend does not store credentials. Cloud credentials live in PocketBase and are referenced by ID.
4. The frontend handles user input validation for UX, but the backend always re-validates. Trust nothing from the client.
5. The frontend uses the established status vocabulary: `pending | deploying | running | failed | rolled_back`. New states require backend and frontend coordination.
6. The frontend uses the established WebSocket message types. New message types require backend and frontend coordination.
7. The frontend respects the design system. New components match the existing visual language.

---

## How To Handle Background Jobs

Background jobs (the reconciliation service, auto-update poller, cost recalculator) follow specific rules:

1. Every background job has a configurable interval. No hardcoded sleeps.
2. Every background job has a kill switch. The job can be disabled without restarting the service.
3. Every background job is idempotent. Running it twice produces the same result as running it once.
4. Every background job has timeout protection. A job that exceeds its expected duration is cancelled and logged.
5. Every background job records its execution in the audit log: started, completed, failed, with timing.
6. Every background job respects rate limits. Cloud API limits, registry pull limits, AI token limits.
7. Every background job is observable. Metrics emit on every cycle: success count, failure count, duration.

---

## How To Handle Tests

Tests are part of the code. The rules:

1. Every new function in the providers, reconciler, or cost packages has at least one unit test.
2. Every public API endpoint has an integration test.
3. Every WebSocket message type has a contract test against the frontend.
4. Every migration has a test that verifies it runs against the prior schema state.
5. Tests do not call real cloud APIs by default. They mock the Provider interface.
6. Tests that do call real cloud APIs are tagged `// +build integration` and only run in the integration CI job.
7. Tests do not use real credentials in any form. They use mock credentials that match the expected format.
8. Test data is isolated per test. There is no shared mutable state between tests.

If you write code without tests, the work is not complete. See `TEST_STRATEGY.md` for more.

---

## How To Handle the Reconciliation Service

The cloud reconciliation service is the heartbeat of cloud deployment management. The rules:

1. The reconciliation loop runs every 30 seconds by default. This is configurable.
2. Each reconciliation cycle processes one deployment target at a time. There is no parallelism inside a single deployment's reconciliation.
3. Multiple deployments can be reconciled in parallel across goroutines, with a configurable concurrency limit.
4. Every reconciliation cycle writes a result to the audit log: success, no-op, drift-detected, error.
5. Drift is detected by comparing desired state (PocketBase) to actual state (cloud API). The diff is recorded.
6. Drift remediation does not happen automatically without user configuration. By default, drift is surfaced to the user.
7. Reconciliation respects cloud API rate limits. If a rate limit is hit, the reconciler backs off exponentially.
8. Reconciliation respects the user's account quotas. If a quota is hit, the affected deployment enters a `quota_exceeded` status and the user is notified.

The reconciliation service is the cloud equivalent of the Kubernetes operator. The same discipline applies.

---

## How To Handle Confidence Calibration

When you describe what you did, calibrate your confidence honestly:

- "I verified X by reading the test output" is honest.
- "I believe X" is honest if you are not certain.
- "X works" without verification is not honest.
- "This should work" is a warning sign that you have not verified.
- "I tested this" is only true if you actually ran the test and saw it pass.

If you did not verify something, say so. If you assumed something, say so. If you did not read a file but think you remember what it contains, say so. False confidence is more dangerous than admitted uncertainty.

---

## How To Handle Long Sessions

Long sessions cause AI agents to forget context. The rules:

1. Re-read `CLAUDE.md` at the start of any work session.
2. Re-state the current task back to the user before starting if the session has been long.
3. If you find yourself uncertain about a recent decision, ask. Do not guess based on what you "remember."
4. If a session has been continuing for hours and you are about to make a destructive change, stop and confirm.
5. If you are about to claim something is done that you started earlier in the session, verify it is actually done.

The dangerous moment is the moment when you assume continuity of memory that you do not actually have.

---

## How To Handle Code Review Comments

When responding to code review feedback on your work:

1. Read the comment fully before responding.
2. If the comment points to a real issue, acknowledge it and fix it.
3. If the comment is asking for clarification, provide it.
4. If the comment is suggesting an alternative approach, evaluate the alternative honestly. If the alternative is better, take it. If your approach is better, explain why with specifics.
5. Do not argue from authority. Arguments are evaluated on merits.
6. Do not silently dismiss feedback. Every comment gets a response.
7. After making changes in response to feedback, summarize what changed.

Code review is a collaboration. Treat it as such.

---

## How To Handle Confusion

If you are confused, stop. The wrong responses to confusion are:

- Pretending you understand and proceeding anyway
- Writing code that looks plausible without verifying
- Generating output that pattern-matches what you think the user wants
- Using vague language to mask uncertainty

The right response to confusion is to state it clearly: "I am confused about X. Specifically, I do not understand Y. Can you clarify Z?"

Confusion in infrastructure code is not a weakness. It is an early warning. Honoring confusion prevents outages.

---

## How To Handle Time Pressure

The user may communicate time pressure. They may say "this is urgent" or "we need this done quickly." The rules:

1. Time pressure never overrides the safety rules in this document.
2. Time pressure never overrides the "stop and ask" rule.
3. Time pressure never justifies skipping tests.
4. Time pressure never justifies skipping documentation.
5. Time pressure is a signal to break the work into smaller, faster pieces — not to skip safety steps.

The slowest path is the one where you cause an outage and have to roll back, debug, and fix forward. The fastest path is the disciplined path.

---

## How To Handle Changes to These Rules

This file changes only with explicit, documented decisions. The process:

1. The proposed change is described in a `DECISIONS.md` entry.
2. The rationale is documented.
3. The user approves the change.
4. The change is applied.

You do not modify this file as a side effect of other work. You do not "improve" the rules. The rules change only through explicit governance.

---

## What "Done" Looks Like

A task is done when:

1. The code change is made.
2. The relevant tests are passing.
3. The relevant documentation is updated.
4. The change has been verified against the actual behavior (not assumed to work).
5. The user has reviewed and confirmed the change.
6. Any new decisions are recorded in `DECISIONS.md`.
7. Any new issues discovered are recorded in `KNOWN_ISSUES.md`.

Anything less than this is not done. It is in progress.

---

## The Final Word

These rules exist because mistakes in infrastructure are expensive and persistent. A bug in a website causes a brief inconvenience. A bug in deployment infrastructure causes downtime for every deployed application across every customer. The scope of damage is multiplied.

You are not less capable for following rules. You are more trustworthy. Following these rules is the difference between code that ships and code that ships safely. Both matter, but only one of them keeps the system alive.

When in doubt: stop, read, ask. The user would rather wait five minutes for clarification than spend five hours fixing an outage.

The Kubernetes system works. Keep it working. Build the cloud system additively. Document everything. Verify everything. The discipline is the product.
