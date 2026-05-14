# CLAUDE.md — AutoStack AI Operating Manual

> **THIS FILE IS READ FIRST. BEFORE TOUCHING ANY CODE, ANY FILE, ANY DECISION.**
> If you are an AI agent working in this repository, read this entire file before doing anything else.

---

## What AutoStack Is

AutoStack is a **production-grade, unified deployment and management platform**.

It allows developers and platform teams to:
- Deploy containerized applications to **Kubernetes clusters** (already working, do not break)
- Deploy containerized applications to **cloud providers** (AWS ECS Fargate, Google Cloud Run, Azure ACA) — this is being built
- Manage deployments with **real-time observability** (logs, metrics, events via WebSocket)
- Automate updates through **image polling and rollout policies**
- Control infrastructure with **blueprint templates** and versioned rollout history
- Estimate and track **cloud costs** before and after deployment
- Operate with **AI-assisted incident explanation** and right-sizing recommendations

The Kubernetes deployment system is **complete and production-ready**. Cloud provider support is **in active development**.

---

## The Single Most Important Rule

```
THE KUBERNETES SYSTEM WORKS.
DO NOT BREAK IT.
DO NOT REFACTOR IT WITHOUT EXPLICIT INSTRUCTION.
DO NOT CHANGE THE CRD SCHEMA WITHOUT EXPLICIT INSTRUCTION.
DO NOT TOUCH THE OPERATOR UNLESS SPECIFICALLY ASKED.
```

Everything cloud-related is **additive**. New files. New collections. New services. Nothing replaces existing Kubernetes functionality. If you are ever unsure whether a change touches the Kubernetes path, **stop and ask**.

See `KUBERNETES_EXISTING_SYSTEM.md` for the full map of what must never be disturbed.

---

## Repository Identity

- **Backend**: Go — Kubernetes operator pattern, PocketBase server, WebSocket hub, background reconciliation service
- **Frontend**: SvelteKit — real-time dashboard, deployment controls, log streaming
- **Database**: PocketBase (SQLite-backed, migrating to PostgreSQL path available but not yet executed)
- **Orchestration**: Custom Kubernetes CRD `one-click.dev/v1alpha1 Rollout` — the operator watches this
- **Auth**: PocketBase built-in auth (OAuth2 social login) — SSO/SAML is planned, not yet built
- **Primary CRD Group**: `one-click.dev/v1alpha1`
- **Operator Namespace**: `autostack` (assumed — confirm before changing)
- **Frontend Port**: varies by environment (see ENVIRONMENT_MATRIX.md)
- **Backend Port**: varies by environment (see ENVIRONMENT_MATRIX.md)

---

## Critical Priorities (In Order)

1. **Working Kubernetes system stays working.** This is non-negotiable.
2. **PocketBase is the single source of truth** for all deployment desired state — both Kubernetes and cloud.
3. **Security is never an afterthought.** Credentials are never logged. Secrets are never stored in plaintext. See `SECURITY_AND_ACCESS.md`.
4. **Cloud deployments are additive.** New provider implementations live behind the Provider interface. No cloud-specific `if provider == "aws"` logic in core paths.
5. **Cost estimates are honest ranges, never promises.** Never hardcode pricing. Always call pricing APIs. Always show uncertainty.

---

## AI Behavioral Rules

### Before You Touch Anything
- Read the relevant section of `ARCHITECTURE.md` for the area you are working in
- Read `SYSTEM_BOUNDARIES.md` to understand what layer owns what
- Read `DATA_MODEL.md` if you are changing any PocketBase collection or schema
- Read `KNOWN_ISSUES.md` so you don't fix things that are intentionally deferred
- Read `DECISIONS.md` so you don't reintroduce patterns that were deliberately rejected

### While Working
- Make **incremental, targeted changes**. Never rewrite a file unless explicitly asked.
- If you are modifying a file, understand every function in it first.
- If you add a dependency, add it to `DECISIONS.md` with rationale.
- If you make an architectural decision (even small), document it.
- Never guess at schema fields. Look at the actual PocketBase collection definitions.
- Never assume a port, URL, or environment variable. Check `ENVIRONMENT_MATRIX.md`.

### When You Are Uncertain
- **Stop and ask.** Uncertainty in infrastructure code causes outages.
- State what you know, what you don't know, and what you need clarified.
- Never silently make an assumption about security, credentials, or destructive operations.

### Things You Must Never Do Without Explicit Human Approval
- Delete or modify PocketBase collections that the Kubernetes system uses
- Change the CRD schema (`one-click.dev/v1alpha1`)
- Modify the Kubernetes operator reconciliation loop
- Change authentication logic
- Add or remove encrypted fields in credential storage
- Modify rollout history or audit log logic
- Execute any infrastructure destroy operation
- Change RBAC permissions on the operator

---

## Repository Conventions

### Folder Structure (Top-Level)
```
/                           → root
/cmd/                       → Go entrypoints (operator, server, reconciler)
/pkg/                       → Go packages
  /pkg/providers/           → Cloud provider implementations (Provider interface)
  /pkg/kubernetes/          → Kubernetes operator logic (DO NOT TOUCH without instruction)
  /pkg/reconciler/          → Cloud reconciliation service
  /pkg/cost/                → Cost estimation and pricing API clients
  /pkg/ai/                  → AI feature integration (incident explainer, right-sizer)
  /pkg/notifications/       → Notification dispatch (Novu integration)
  /pkg/secrets/             → Secret management abstraction
  /pkg/registry/            → Container registry credential management
/internal/                  → Internal-only packages (not exported)
/api/                       → API route definitions
/frontend/                  → SvelteKit application
/docs/                      → All documentation files (this folder)
/helm/                      → Helm chart for operator installation
/deploy/                    → Deployment manifests
/scripts/                   → Automation scripts
```

### Naming Conventions
- Go files: `snake_case.go`
- Go types: `PascalCase`
- Go functions: `PascalCase` (exported), `camelCase` (unexported)
- PocketBase collections: `snake_case` (e.g., `cloud_accounts`, `deployment_targets`)
- Environment variables: `SCREAMING_SNAKE_CASE` with `AUTOSTACK_` prefix
- SvelteKit routes: `lowercase-hyphenated`
- API endpoints: `lowercase-hyphenated` under `/api/v1/`

### Forbidden Patterns
- No hardcoded cloud pricing values anywhere in the codebase
- No raw AWS/GCP/Azure SDK calls outside of `/pkg/providers/`
- No business logic in SvelteKit frontend (UI only, all logic in backend API)
- No secrets or credentials in logs at any log level
- No `fmt.Println` in production Go code (use structured logger)
- No direct PocketBase SQL queries outside the data access layer
- No `cluster-admin` assumptions in any new Kubernetes RBAC

---

## Dangerous Operation Policies

| Operation | Policy |
|---|---|
| Delete cloud resources | Requires user confirmation in UI + audit log entry |
| Delete Kubernetes deployment | Requires user confirmation in UI + audit log entry |
| Rotate cloud credentials | Requires re-validation after rotation |
| Force delete (bypass cloud cleanup) | Requires explicit user override with warning |
| Change operator RBAC | Requires human review + version bump |
| PocketBase schema migration | Requires migration file, cannot be manual |
| Production deployment (of AutoStack itself) | Requires staging validation first |

---

## Validation Expectations

Before marking any task complete:
- The change does not break existing Kubernetes functionality (verify by checking the operator codepath is unchanged)
- New cloud provider code is behind the Provider interface, not inline
- No credentials or secrets appear in any logging statement
- If PocketBase schema changed, a migration file exists
- If a new environment variable is required, it is added to `ENVIRONMENT_MATRIX.md`
- If a new dependency was added, it is documented in `DECISIONS.md`

---

## Engineering Philosophy

**Simplicity over cleverness.** A function that is easy to read is better than one that is elegant but opaque.

**Operational reliability over features.** A broken deployment that shows a clear error message is better than a broken deployment that silently fails.

**Honest cost estimates over impressive estimates.** Show a range. Show uncertainty. Never promise a number that will surprise the user.

**The working Kubernetes system is the anchor.** All new features are designed around the same patterns (reconciliation loops, single source of truth, WebSocket real-time updates) that made Kubernetes management work.

**Cloud deployments must feel identical to Kubernetes deployments from the user's perspective.** Same UI patterns. Same status vocabulary. Same rollback experience. Different execution under the hood.

---

## How to Onboard to a New Task

1. Read this file (done)
2. Read `ARCHITECTURE.md` → understand which layer your task touches
3. Read `SYSTEM_BOUNDARIES.md` → understand what is and is not your responsibility
4. Read `KNOWN_ISSUES.md` → understand what is already known to be broken or deferred
5. Read the relevant subsection of `DATA_MODEL.md` if schema is involved
6. Read `DECISIONS.md` → understand decisions that affect your area
7. Begin work with incremental, targeted changes
8. Update `DECISIONS.md` if you make a new architectural decision
9. Update `KNOWN_ISSUES.md` if you discover a new issue but are not fixing it now

---

## The North Star

> A developer with a Docker image and no cloud expertise should be able to deploy to production, with monitoring, cost visibility, automatic updates, and rollback capability, in under 10 minutes.

Every feature decision is evaluated against this north star.
