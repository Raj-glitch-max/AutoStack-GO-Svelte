# DECISIONS.md — Architecture Decision Record (ADR)

> **This file is the authoritative record of every significant decision made about the AutoStack architecture, technology stack, and design.** It exists so that future contributors (human and AI) understand not just what the system does, but *why* it was built that way. A decision recorded here is harder to accidentally reverse than an undocumented decision. This file grows over time and should never have entries deleted.

---

## How To Read This File

Each decision uses the following structure:

```
### ADR-NNN: Short title
- Date: YYYY-MM-DD
- Status: [proposed | accepted | superseded by ADR-XXX | deprecated]
- Decision makers: [names or "core team"]
- Context: [what situation prompted this decision]
- Decision: [the actual decision]
- Consequences: [what becomes true because of this decision]
- Alternatives considered: [other options that were rejected]
- Rationale: [why this option was chosen]
```

When a decision is reversed or evolved, the new ADR references the old one, and the old one is marked `superseded by ADR-XXX`. Old ADRs are never deleted. They show the evolution of thinking.

---

## Foundational Decisions

### ADR-001: PocketBase as the database and auth layer
- Date: Project inception (Kubernetes phase)
- Status: accepted
- Decision makers: original author
- Context: The original AutoStack needed a database, auth system, file storage, and admin interface. The team was small (one person). Operational overhead had to be minimal.
- Decision: Use PocketBase as the unified database, auth, and file storage layer.
- Consequences:
  - SQLite-backed storage. Suitable for single-node deployments. Scales by vertical scaling, not horizontal.
  - PocketBase admin UI provides immediate access to data inspection and modification.
  - Migrations are written in JavaScript (a PocketBase requirement).
  - Auth includes built-in OAuth2 social providers.
  - File storage is local-disk by default (with S3 backend optional).
- Alternatives considered:
  - PostgreSQL + custom auth (rejected: too much custom work for early stage).
  - Firebase / Supabase (rejected: vendor lock-in, less control).
  - SQLite + Lucia auth (rejected: still requires custom admin UI).
- Rationale: PocketBase eliminates an entire category of infrastructure work in early stage. As scale demands grow, migration to PostgreSQL is a known path. The cost of migration is bounded.

### ADR-002: Kubernetes operator pattern via custom CRD
- Date: Project inception (Kubernetes phase)
- Status: accepted
- Decision makers: original author
- Context: The original AutoStack needed a way to manage Kubernetes deployments declaratively, with watch-based reconciliation rather than polling.
- Decision: Define a custom CRD `Rollout` in group `one-click.dev/v1alpha1` and run a custom Kubernetes operator that reconciles `Rollout` resources to the underlying Kubernetes resources (Deployment, Service, Ingress, etc.).
- Consequences:
  - The system gets all of Kubernetes' built-in features: watch, event-driven reconciliation, leader election, status subresource.
  - Users can interact with the CRD directly via `kubectl` if they want.
  - The system is composable with other Kubernetes-native tools.
  - Operator must be installed in each target cluster.
  - The CRD schema becomes a long-term contract.
- Alternatives considered:
  - Direct Kubernetes API calls without CRD (rejected: no reconciliation, no recovery from drift).
  - Helm chart per deployment (rejected: complex, hard to make dynamic).
- Rationale: The operator pattern is the canonical Kubernetes way to extend the platform. It gives reconciliation and recovery for free. The investment in defining a CRD pays off long-term.

### ADR-003: SvelteKit for the frontend
- Date: Project inception
- Status: accepted
- Decision makers: original author
- Context: A modern web framework was needed for the dashboard. Real-time updates via WebSocket were a hard requirement. The team had limited frontend bandwidth.
- Decision: Use SvelteKit with TypeScript.
- Consequences:
  - Smaller bundle sizes than React or Vue equivalents.
  - Reactive stores integrate well with WebSocket message streams.
  - Server-side rendering option available if needed (currently not used).
- Alternatives considered:
  - React + Vite (rejected: heavier bundle, more boilerplate for reactive state).
  - Vue 3 + Vite (rejected: comparable but team has more Svelte experience).
  - Solid.js (rejected: smaller ecosystem at the time).
- Rationale: SvelteKit produces small, fast frontends with good developer ergonomics. The reactive store model maps cleanly to WebSocket-driven UI updates.

### ADR-004: Go for the backend
- Date: Project inception
- Status: accepted
- Decision makers: original author
- Context: The backend needs to interact with Kubernetes (Go is the native ecosystem), do background reconciliation (concurrency primitives needed), and serve a WebSocket API. Performance and operational simplicity matter.
- Decision: Use Go for all backend services.
- Consequences:
  - Single binary deployment.
  - Strong typing and compile-time safety.
  - Native concurrency for reconciliation loops.
  - Direct use of `client-go` for Kubernetes interaction.
  - Slower iteration than dynamic languages, but more reliability.
- Alternatives considered:
  - Node.js (rejected: weaker concurrency model, less native Kubernetes support).
  - Rust (rejected: steeper learning curve, less Kubernetes ecosystem).
  - Python (rejected: slower runtime, less native Kubernetes ecosystem).
- Rationale: Go is the lingua franca of Kubernetes. The ecosystem (client-go, controller-runtime, operator-sdk) is mature.

---

## Cloud Integration Decisions

### ADR-005: Provider interface for cloud abstractions
- Date: Multicloud planning phase
- Status: accepted
- Decision makers: core team
- Context: AutoStack needs to support multiple cloud providers (AWS, GCP, Azure) plus Kubernetes. Each has different APIs, different concepts, different lifecycle quirks. Implementing each cloud's logic inline would create a maintenance nightmare.
- Decision: Define a Go interface `Provider` in `/pkg/providers/provider.go` with methods for the deployment lifecycle: `ValidateCredentials`, `Deploy`, `GetStatus`, `GetMetrics`, `StreamLogs`, `EstimateCost`, `GetActualCost`, `Destroy`, `ListRegions`. Each cloud provider implements this interface in its own package.
- Consequences:
  - All cloud-specific logic is isolated to `/pkg/providers/<cloud>/`.
  - The core deployment service is provider-agnostic.
  - Adding a new cloud provider is bounded work: implement the interface, register it.
  - The interface itself becomes a long-term contract that is hard to change.
- Alternatives considered:
  - Inline cloud-specific code with `if provider == "aws"` branches (rejected: unmaintainable at scale).
  - Plugin system with dynamically loaded providers (rejected: over-engineering for current needs).
  - Adopt an existing abstraction like Pulumi or Crossplane (rejected: too heavy, too opinionated).
- Rationale: A Go interface is the simplest, most idiomatic abstraction for this. It compiles. It's typed. It's testable. It avoids over-engineering.

### ADR-006: PocketBase is the single source of truth (not the CRD)
- Date: Multicloud planning phase
- Status: accepted
- Decision makers: core team
- Context: With cloud providers, there is no CRD equivalent in cloud. ECS has no analog to the Kubernetes `Rollout` CRD. The question is: where does the desired state of a deployment live? In the cluster (CRD) or in PocketBase?
- Decision: PocketBase is the single source of truth for all deployment desired state, both Kubernetes and cloud. For Kubernetes, the CRD is generated from PocketBase records. The operator reconciles from the CRD. But PocketBase is the source.
- Consequences:
  - All deployments — Kubernetes and cloud — have a uniform representation in PocketBase.
  - Rollout history, blueprints, and audit logs are all centralized.
  - The CRD becomes a derived artifact, not the source.
  - Multi-cluster Kubernetes (where the same deployment goes to multiple clusters) becomes possible.
  - There is a synchronization concern: PocketBase changes must be propagated to the CRD.
- Alternatives considered:
  - CRD is truth for Kubernetes, PocketBase is truth for cloud (rejected: split brain, inconsistent UX).
  - CRD is truth everywhere with cloud "fake CRDs" (rejected: too much fiction).
- Rationale: Single source of truth simplifies reasoning. The synchronization cost is bounded and the operator already runs reconciliation. The current implementation already syncs PocketBase to CRD; this decision formalizes that pattern.

### ADR-007: Reconciliation service for cloud deployments
- Date: Multicloud planning phase
- Status: accepted
- Decision makers: core team
- Context: Kubernetes deployments have an operator with built-in reconciliation. Cloud deployments need an equivalent.
- Decision: Build a separate reconciliation service (`/pkg/reconciler/`) that runs as a background goroutine in the backend. It polls each cloud deployment on a configurable interval (default 30 seconds), compares actual state to desired state, and takes corrective action when drift is detected.
- Consequences:
  - Cloud deployments get the same recovery-from-drift behavior as Kubernetes.
  - Reconciliation interval affects cloud API call volume (rate limits, costs).
  - The reconciler is a critical-path background service. If it stops running, cloud deployments stop being managed.
  - Need leader election if AutoStack runs multiple replicas.
- Alternatives considered:
  - Event-driven via cloud webhooks (rejected: not all providers offer webhooks; rate limits worse).
  - On-demand reconciliation triggered by user actions only (rejected: doesn't catch drift).
- Rationale: Polling is the only universally-supported pattern. 30 seconds is a balance between drift detection latency and API call volume.

### ADR-008: Cloud Run as the first non-Kubernetes provider
- Date: Multicloud planning phase
- Status: accepted
- Decision makers: core team
- Context: The first cloud provider implementation determines a lot — sets patterns, exposes interface gaps, validates the architecture. The choice should be the one that exposes the most patterns with the least incidental complexity.
- Decision: Implement Google Cloud Run as the first non-Kubernetes provider, before AWS ECS Fargate.
- Consequences:
  - Cloud Run has minimal networking decisions (no VPC required by default).
  - Cloud Run's cold start behavior surfaces the "serverless-style scaling" UX issues we will face with ACA too.
  - Cloud Run is cheap for testing.
  - The first user-visible "cloud deployment" will be a Cloud Run deployment.
- Alternatives considered:
  - AWS ECS Fargate first (rejected: more incidental complexity from VPC requirements).
  - Azure ACA first (rejected: smaller user base, less reference material).
- Rationale: Cloud Run is the cleanest serverless container target. It exposes the patterns we need to support without forcing premature networking decisions.

### ADR-009: Per-user AI provider API keys, not platform-paid
- Date: AI planning phase
- Status: accepted
- Decision makers: core team
- Context: AI features (incident explanation, right-sizing, Compose conversion) require calling an AI provider. The platform can either pay for these calls (and price it into the platform fee) or require the user to bring their own AI API key.
- Decision: AutoStack does not pay for AI calls. Users configure their own AI provider (Anthropic, OpenAI, Azure OpenAI, etc.) and provide their own API key.
- Consequences:
  - AutoStack has zero AI operational cost.
  - Users with no AI key can still use AutoStack; AI features are opt-in.
  - Enterprise users with data residency requirements can use Azure OpenAI in their own region.
  - Users see and control their AI spending directly.
- Alternatives considered:
  - Platform pays for AI, prices it into the subscription (rejected: high variable cost risk, complex margin math).
  - Hybrid: a small free tier of AI, then user-paid (rejected: complex to implement, customer education burden).
- Rationale: User-paid AI removes a hard cost ceiling for AutoStack and gives users control over their AI usage and provider choice.

### ADR-010: Cost estimates always shown as ranges with stated assumptions
- Date: Cost intelligence planning
- Status: accepted
- Decision makers: core team
- Context: Cost estimates that look like single numbers create unrealistic expectations. Users see "$50/month" and assume their bill will be $50. When the bill is $200 (data transfer, NAT gateway, etc.), trust is broken.
- Decision: Cost estimates are always displayed as ranges (e.g., "$45 - $80 per month") with explicit, visible assumptions ("assumes 100 GB/month data transfer, no NAT gateway data, standard logging volume"). Single-number estimates are forbidden.
- Consequences:
  - The UI must always render a range.
  - Backend cost calculation must always produce a range.
  - Assumption text must be human-readable and accurate.
  - Some users will find this less satisfying than a confident single number — but trust is more important.
- Alternatives considered:
  - Single estimate with "approximate" disclaimer (rejected: still creates the same expectation).
  - Three-tier estimate: low / typical / high (rejected: more complex, similar value).
- Rationale: Honesty about uncertainty is non-negotiable. Users can plan for ranges. Users cannot plan for surprises.

---

## Technology Stack Decisions

### ADR-011: WebSocket for real-time, REST for everything else
- Date: Initial architecture
- Status: accepted
- Decision makers: original author
- Context: The dashboard needs real-time updates (deployment status, logs, metrics). The rest of the API is request-response.
- Decision: Use WebSocket for streaming data (logs, status updates, metrics points). Use REST for everything else (CRUD operations, queries).
- Consequences:
  - Two API surfaces to maintain.
  - WebSocket connections require careful lifecycle management.
  - Some operations could be either; documented choice resolves ambiguity.
- Alternatives considered:
  - All-WebSocket API (rejected: REST is simpler for CRUD, better tooling).
  - Server-Sent Events for streaming (rejected: WebSocket is more widely supported and bidirectional).
  - GraphQL subscriptions (rejected: over-engineering for our needs).
- Rationale: WebSocket where you need bidirectional streaming, REST where you don't. Match the tool to the job.

### ADR-012: TypeScript on the frontend
- Date: Initial architecture
- Status: accepted
- Decision makers: original author
- Context: SvelteKit supports both JavaScript and TypeScript. The frontend will have non-trivial state, complex props, and many API integration points.
- Decision: All frontend code is TypeScript. No `any` types except where unavoidable.
- Consequences:
  - Compile-time safety for frontend code.
  - Refactoring is much safer.
  - Slight learning curve for contributors more familiar with JavaScript.
- Alternatives considered:
  - Plain JavaScript with JSDoc types (rejected: weaker enforcement).
- Rationale: Frontend type safety prevents a class of runtime bugs that are hard to catch in tests.

### ADR-013: No state management library on the frontend (use Svelte stores)
- Date: Initial architecture
- Status: accepted (may revisit per ISSUE-027)
- Decision makers: original author
- Context: The frontend has reactive state. Options include using Svelte's built-in stores, adopting Redux/Zustand-style state management, or building a custom solution.
- Decision: Use Svelte's built-in stores (`writable`, `readable`, `derived`). No external state management library.
- Consequences:
  - Less complexity.
  - State logic is colocated with components.
  - As the application grows, this may need revisiting (see ISSUE-027).
- Alternatives considered:
  - Redux Toolkit (rejected: heavy for our needs).
  - Zustand (rejected: not idiomatic in Svelte).
- Rationale: Svelte stores are sufficient for current scope. Don't add dependencies for hypothetical needs.

### ADR-014: PocketBase migrations checked into version control
- Date: Initial architecture
- Status: accepted
- Decision makers: original author
- Context: PocketBase migrations need to be applied consistently across all environments (dev, staging, production).
- Decision: All migration files are checked into the repository under `/migrations/`. Migrations are applied automatically on backend startup. Migration files are immutable once merged.
- Consequences:
  - Schema changes are tracked in git history.
  - Migrations apply automatically on deployment.
  - A bad migration must be reversed by a new migration, not by editing the old one.
- Alternatives considered:
  - Manual migrations via PocketBase admin UI (rejected: not reproducible across environments).
  - Out-of-band migration tool (rejected: more moving parts).
- Rationale: Auto-applied versioned migrations are the standard pattern. Immutability is the key invariant.

---

## Security Decisions

### ADR-015: Credentials encrypted at rest with AES-256-GCM
- Date: Security baseline
- Status: accepted
- Decision makers: core team
- Context: Cloud provider credentials, registry credentials, and other secrets are stored in PocketBase. They must be encrypted at rest.
- Decision: Encrypt sensitive fields with AES-256-GCM using a key loaded from the `AUTOSTACK_ENCRYPTION_KEY` environment variable. Encryption is at the application layer; PocketBase sees ciphertext.
- Consequences:
  - Database-level access does not expose credentials.
  - The encryption key itself is sensitive; see ISSUE-002 about migrating to KMS.
  - Encrypted fields cannot be queried directly. Where filtering is needed, store an unencrypted index field (e.g., `account_name`) alongside the encrypted credentials.
- Alternatives considered:
  - Database-level encryption (rejected: relies on database configuration, less portable).
  - PGP-style asymmetric encryption (rejected: more complex, not necessary for symmetric data).
- Rationale: AES-256-GCM is industry-standard authenticated encryption. The application-layer approach is portable.

### ADR-016: No PII in logs, ever
- Date: Security baseline
- Status: accepted
- Decision makers: core team
- Context: Logs are aggregated, retained, and may be accessed by support and engineering. PII in logs creates compliance risk.
- Decision: Personal data (email addresses, full names, IP addresses beyond /24 truncation, payment data, cloud credentials) is never logged. The structured logger has a built-in PII filter that redacts known PII fields.
- Consequences:
  - Some debug scenarios are harder because logs don't carry full context.
  - Audit trail (which is intentional and authorized) is a separate system from logs.
  - Compliance posture is significantly stronger.
- Alternatives considered:
  - Log everything, scrub at retention (rejected: PII might leak to log aggregators).
- Rationale: The principle is "don't log what you don't want stolen." Stronger than retention-based scrubbing.

### ADR-017: OAuth2 for authentication, SAML/SCIM planned
- Date: Auth baseline
- Status: accepted
- Decision makers: core team
- Context: User authentication is a first-day concern. The choice of authentication mechanism affects what kinds of users can use the platform.
- Decision: PocketBase OAuth2 (GitHub, Google, Microsoft) is the initial auth mechanism. SAML and SCIM are planned for enterprise customers in Phase 2.
- Consequences:
  - Developers can sign up with one click using GitHub.
  - Enterprise customers cannot enforce SSO until SAML lands.
  - Magic link / email-password is also available as a fallback.
- Alternatives considered:
  - WorkOS or Clerk from day 1 (rejected: cost, complexity, vendor lock-in before validating market).
  - Build SAML from scratch first (rejected: SAML is complex, premature for current user base).
- Rationale: OAuth2 covers developer signup. SAML follows enterprise demand.

### ADR-018: Audit log is append-only, separate from operational logs
- Date: Compliance planning
- Status: accepted
- Decision makers: core team
- Context: SOC2 and similar frameworks require an audit log of sensitive operations. This log must be immutable and accessible to authorized auditors.
- Decision: The audit log is its own PocketBase collection (`audit_log`). It has no update or delete API. Entries can only be inserted. It captures: actor, action, resource type, resource id, timestamp, IP, user agent, request id, before/after state diff (for changes), result.
- Consequences:
  - The audit log is separate from regular application logs.
  - Audit log queries are read-only.
  - Audit log retention is longer than operational log retention (default 7 years).
- Alternatives considered:
  - Use operational logs for audit (rejected: operational logs are mutable, deletable).
  - External audit log service (rejected: more complexity, less control).
- Rationale: Audit logs are a distinct concern with distinct requirements. Treating them separately is the standard pattern.

---

## Operational Decisions

### ADR-019: Helm chart for AutoStack installation
- Date: Distribution planning
- Status: accepted
- Decision makers: core team
- Context: AutoStack itself runs on Kubernetes. Users need a way to install it.
- Decision: Distribute AutoStack as a Helm chart. The chart includes: the backend deployment, the frontend deployment, the operator deployment, the PocketBase StatefulSet, optional ingress configuration.
- Consequences:
  - Standard Kubernetes installation experience.
  - Configuration through `values.yaml`.
  - Versioning aligned with Helm chart versions.
  - Upgrades follow Helm conventions (`helm upgrade`).
- Alternatives considered:
  - Kustomize-based installation (rejected: less common for distributed software).
  - Operator-based installation (rejected: chicken-and-egg with the AutoStack operator).
- Rationale: Helm is the standard. Users know how to operate Helm-installed software.

### ADR-020: Single-binary backend, multiple goroutines for services
- Date: Initial architecture
- Status: accepted
- Decision makers: original author
- Context: The backend has multiple services: HTTP API, WebSocket server, reconciler, auto-update poller, cost recalculator. These could be separate binaries or one.
- Decision: Single Go binary for the backend. Each service runs as a goroutine started by the main function. Services are configurable (can be disabled per replica via environment variables).
- Consequences:
  - One artifact to build, ship, deploy.
  - Shared resources (DB connections, HTTP clients) across services.
  - When running multiple replicas, leader election prevents duplicate background work.
  - Operational simplicity for small deployments; can be split later if needed.
- Alternatives considered:
  - Microservices from day one (rejected: premature, operational overhead).
- Rationale: Monolith first. Split later if the boundaries become clear and justified.

### ADR-021: Leader election for background services
- Date: HA planning
- Status: accepted
- Decision makers: core team
- Context: When AutoStack runs as multiple replicas (for HA), background services (reconciler, auto-update poller, cost recalculator) must not run on every replica or they will do duplicate work and may corrupt state.
- Decision: Use Kubernetes leader election (via `client-go/tools/leaderelection`) for background services. Only the leader runs background tasks. The HTTP/WebSocket API runs on all replicas.
- Consequences:
  - Background work happens on one replica at a time.
  - Failover happens automatically if the leader fails.
  - There is a brief window during failover where background work pauses.
  - Leader election requires Kubernetes (or another lease mechanism for non-K8s deployments).
- Alternatives considered:
  - Database-based locking (rejected: harder to fail over cleanly).
  - One specific replica is always the leader (rejected: not actually HA).
- Rationale: Kubernetes-native leader election is well-understood and battle-tested.

### ADR-022: Reconciliation interval default of 30 seconds, configurable
- Date: Reconciliation design
- Status: accepted
- Decision makers: core team
- Context: How often should the cloud reconciler check each deployment? Too often: high cloud API costs, rate limit risk. Too rarely: slow drift detection.
- Decision: Default reconciliation interval is 30 seconds per deployment. Configurable via environment variable. Each deployment is reconciled once per interval, with a per-provider concurrency limit.
- Consequences:
  - For 100 deployments, that's ~200 API calls per minute. Within rate limits for all major providers.
  - Drift detection latency is up to 30 seconds.
  - User-perceived latency for actions (start, stop, redeploy) is masked because the action itself is synchronous; reconciliation just verifies.
- Alternatives considered:
  - 10 seconds (rejected: 3x API calls for marginal latency benefit).
  - 60 seconds (rejected: drift detection too slow).
  - Adaptive (faster when actively deploying, slower otherwise) (rejected: complex, marginal value).
- Rationale: 30 seconds is a reasonable balance. Configurability handles edge cases.

---

## Provider-Specific Decisions

### ADR-023: AWS Pricing API for compute cost calculation, Infracost for surrounding infra
- Date: Cost intelligence planning
- Status: accepted
- Decision makers: core team
- Context: Cost calculation needs accurate prices. AWS has a Pricing API. Surrounding infra (load balancers, NAT gateways) is harder to model from scratch.
- Decision: For compute cost (vCPU, memory hours), call AWS Pricing API directly with a daily cache. For surrounding infrastructure cost, call the Infracost API. Combine the two for the total estimate range.
- Consequences:
  - Compute cost is precise.
  - Surrounding infra cost is reasonably accurate.
  - Two external dependencies (AWS Pricing API and Infracost).
  - Daily cache reduces API call volume.
- Alternatives considered:
  - Build all cost calculation in-house (rejected: massive ongoing maintenance for prices that change frequently).
  - Use only Infracost (rejected: less precise for compute).
- Rationale: AWS Pricing API is authoritative and free. Infracost handles what AWS Pricing API doesn't cover well.

### ADR-024: ECR pull-through cache recommended for AWS deployments
- Date: AWS provider planning
- Status: accepted
- Decision makers: core team
- Context: ECS Fargate pulls images from container registries. Pulls from external registries (Docker Hub, etc.) incur data transfer cost and may hit rate limits.
- Decision: Recommend (not require) users configure ECR pull-through cache when deploying to ECS Fargate. Document this in the deployment guide. Detect when a deployment is pulling from an external registry without pull-through cache and surface a warning.
- Consequences:
  - Best practice is documented and surfaced.
  - Users can ignore the warning if they prefer.
  - Reduces user frustration when external registries impose rate limits.
- Alternatives considered:
  - Force pull-through cache (rejected: not all users have ECR access).
- Rationale: Recommend, don't require. Surface tradeoffs.

### ADR-025: Cloud Run uses Google Artifact Registry preferred path
- Date: Cloud Run provider planning
- Status: accepted
- Decision makers: core team
- Context: Cloud Run can pull from any registry, but pull performance and reliability is best with Google Artifact Registry.
- Decision: Document Artifact Registry as the preferred registry for Cloud Run deployments. Continue to support other registries with a warning surfaced in the UI.
- Consequences:
  - Best practice documented.
  - Non-Artifact-Registry deployments still work, just with worse pull performance.
- Alternatives considered:
  - Require Artifact Registry (rejected: too restrictive).
- Rationale: Same as ADR-024 — recommend, don't require.

### ADR-026: No support for deploying to user's own Kubernetes from cloud provider integrations
- Date: Scope boundary
- Status: accepted
- Decision makers: core team
- Context: EKS, GKE, and AKS are Kubernetes-on-cloud. The user could deploy to them. Should AutoStack treat them as separate providers, or just as Kubernetes deployments to a managed cluster?
- Decision: EKS, GKE, AKS are not separate providers. They are just Kubernetes clusters managed by the user's cloud account. AutoStack's Kubernetes path supports any conformant Kubernetes cluster. The user configures the cluster connection details directly.
- Consequences:
  - No separate provider implementation needed for EKS/GKE/AKS.
  - User must provision the managed Kubernetes cluster themselves; AutoStack does not create the cluster.
  - The user's Kubernetes path works the same regardless of whether the cluster is on-premise or managed cloud.
- Alternatives considered:
  - Implement EKS, GKE, AKS as separate providers (rejected: duplicates Kubernetes provider work).
- Rationale: A Kubernetes cluster is a Kubernetes cluster. The hosting (on-prem, EKS, GKE, AKS) is the user's choice.

---

## API and Data Decisions

### ADR-027: REST API uses `/api/v1/` prefix
- Date: API design
- Status: accepted
- Decision makers: core team
- Context: API versioning is necessary to evolve the API without breaking existing clients.
- Decision: All REST API endpoints are under `/api/v1/`. Breaking changes will land in `/api/v2/` when they happen. Old versions are maintained per the deprecation policy.
- Consequences:
  - Frontend always calls v1.
  - Future v2 requires parallel support.
  - URLs are predictable.
- Alternatives considered:
  - Header-based versioning (rejected: less discoverable).
  - No versioning (rejected: locked into v1 contract forever).
- Rationale: URL-path versioning is the most common and discoverable convention.

### ADR-028: All API responses use a consistent envelope
- Date: API design
- Status: accepted
- Decision makers: core team
- Context: Different API endpoints return different shapes. This makes client code harder.
- Decision: All API responses use this envelope:
  ```json
  {
    "ok": true | false,
    "data": <the response payload>,
    "error": {
      "code": "...",
      "message": "...",
      "details": {...}
    } | null,
    "meta": {
      "request_id": "...",
      "timestamp": "..."
    }
  }
  ```
- Consequences:
  - Predictable client code.
  - Always have a request ID for debugging.
  - Errors are structured, not strings.
- Alternatives considered:
  - Use HTTP status codes only (rejected: insufficient for structured errors).
  - JSON:API spec (rejected: too prescriptive for our needs).
- Rationale: Consistent envelope is one of the highest-leverage decisions for API ergonomics.

### ADR-029: Pagination is offset-based for now, cursor-based when needed
- Date: API design
- Status: accepted
- Decision makers: core team
- Context: List endpoints need pagination. Offset-based is simpler but breaks if the underlying data changes during paging. Cursor-based is more robust but harder to implement.
- Decision: Use offset/limit pagination (`?page=N&limit=M`) for list endpoints. Migrate to cursor-based pagination for endpoints where data churn is high (e.g., audit log queries).
- Consequences:
  - Simple to implement initially.
  - Some endpoints will eventually need cursor-based.
  - Document which endpoints use which.
- Alternatives considered:
  - Cursor-based from day one (rejected: premature for low-data-churn endpoints).
- Rationale: Pick the simpler tool. Upgrade where the simpler tool breaks down.

### ADR-030: Resources are identified by stable IDs, not by name
- Date: Data model
- Status: accepted
- Decision makers: core team
- Context: Resources (deployments, projects, etc.) need identifiers. Options: human-readable names, UUIDs, slugs.
- Decision: Resources have an internal stable ID (PocketBase auto-generated). They also have a human-readable name. API references use the ID, not the name. The UI shows the name; the URL uses the ID.
- Consequences:
  - Names can be changed without breaking references.
  - URLs are not as readable.
  - IDs are stable across renames.
- Alternatives considered:
  - Name-based references (rejected: rename breaks all references).
  - Slug-based references (rejected: complex slug generation, collision handling).
- Rationale: Stable IDs are the right primitive. Names are user-facing.

---

## Frontend Decisions

### ADR-031: Tailwind CSS for styling
- Date: Frontend baseline
- Status: accepted
- Decision makers: original author
- Context: Choosing a styling approach: CSS modules, Tailwind, styled-components, vanilla CSS.
- Decision: Use Tailwind CSS with utility-first conventions.
- Consequences:
  - Fast UI iteration.
  - Smaller CSS bundle (only used utilities are included).
  - Class strings can become long; component extraction is the answer.
- Alternatives considered:
  - CSS Modules (rejected: more boilerplate).
  - styled-components (rejected: runtime overhead, less idiomatic in Svelte).
- Rationale: Tailwind is widely used and produces consistent UIs quickly.

### ADR-032: Flowbite Svelte as the component library
- Date: Frontend baseline
- Status: accepted
- Decision makers: original author
- Context: Building every UI component from scratch is slow. A component library accelerates development.
- Decision: Use Flowbite Svelte for common components (buttons, modals, dropdowns, etc.). Build custom components for product-specific UI.
- Consequences:
  - Consistent baseline UI.
  - Some Flowbite components may need overrides for our design.
  - Dependency on the library; risk if it becomes unmaintained.
- Alternatives considered:
  - Build everything custom (rejected: slow).
  - Skeleton UI (rejected: similar to Flowbite, no strong preference at the time).
- Rationale: Pragmatic choice. We can replace it if needed.

### ADR-033: Monaco Editor for code/YAML/JSON editing in the UI
- Date: Frontend feature
- Status: accepted
- Decision makers: original author
- Context: Users need to edit YAML manifests, JSON configurations, environment variables. A plain textarea is insufficient.
- Decision: Use Monaco Editor (the editor that powers VS Code) for all code/YAML/JSON editing in the UI.
- Consequences:
  - Familiar UX for developers.
  - Syntax highlighting, validation, autocomplete out of the box.
  - Heavier bundle.
- Alternatives considered:
  - CodeMirror (rejected: smaller but less familiar).
  - Plain textarea (rejected: poor UX for structured content).
- Rationale: Developers expect VS Code-like editing. Monaco delivers that.

---

## Phase 2.3 Control-Plane Integrity Decisions

### ADR-034: `deployment_targets.pending_destroy` (bool) for in-flight destroy re-arm
- Decision: Add a `pending_destroy` bool column. When an `endDate` is set on a rollout whose target has `current_operation != ""`, the controller sets `pending_destroy = true` instead of attempting a status flip. The dispatcher's post-Deploy success path checks the flag, routes the target to `deleting` on release, and clears the flag.
- Alternatives considered:
  - Re-read rollout `endDate` from the dispatcher on release (rejected: extra read per release path; conflates rollout fields into the dispatcher).
  - Queue destroy intent in a separate table (rejected: over-engineered for one bit).
  - Block endDate writes while in-flight (rejected: user-hostile; the operator may have a legitimate abort intent).
- Rationale: Closes the "endDate during in-flight deploy silently dropped" hazard with one bit and one branch in the release path.
- Source: `project-context/phase2.3/delete-orphan-risk-assessment.md` §D-1.

### ADR-035: Heartbeat-aware startup sweep
- Decision: Startup sweep ignores in-progress ops whose `operations.updated_at` is within `2 × heartbeatInterval` AND has actually heartbeated at least once (`updated_at > started_at`). All other in-progress ops are abandoned regardless of age.
- Alternatives considered:
  - Keep the original "sweep everything" semantic (rejected: rolling restart of a single replica aggressively kills in-flight ops the outgoing pod could legitimately finish).
  - Use `abandonedOpThreshold` (20 min) as a window (rejected: too generous; a 20-min-old op without heartbeat is more likely dead than draining).
- Rationale: Single-pod rolling restart becomes safe without introducing pod-identity stamping (deferred to Phase 2.5). First-heartbeat guard preserves "process crashed mid-create" abandonment semantics.
- Source: `project-context/phase2.3/ownership-integrity-review.md` §O-2 and `replay-safety-assessment.md` §2.

### ADR-036: `deployment_history.provider` records the canonical (target-side) provider
- Decision: `writeHistory` now records `deployment_targets.provider` (e.g. `gcp-cloudrun`) instead of `cloud_accounts.provider` (e.g. `gcp`). Lineage rows are now consistent with target rows.
- Alternatives considered:
  - Normalize all three enums to one (rejected: deferred to Phase 2.5; large schema work).
  - Leave inconsistency in place (rejected: operators querying by provider get split results).
- Rationale: Cheap fix to remove one source of operator confusion. The single canonical enum effort remains a separate Phase 2.5 item.
- Source: `project-context/phase2.3/lineage-integrity-review.md` §L-1.

### ADR-037: Intent-boundary history rows
- Decision: `createPendingDeploymentTarget`, `flipCloudTargetsToPendingOnRespec`, and `markCloudTargetForDestroy` now write `deployment_history` rows recording operator intent (action + `status=in_progress` + message describing intent). The dispatcher's later in-progress rows remain unchanged.
- Alternatives considered:
  - Skip intent rows; rely on dispatcher claim rows for lineage (rejected: incident-reconstruction gap between "operator did X" and "dispatcher did Y" was too wide).
  - Write intent rows AND skip dispatcher in-progress rows (rejected: dispatcher claim is the load-bearing CAS event; lineage should still record it).
- Rationale: Operators reading history can now answer "when did this start" with the operator's action time, not the reconciler's tick time.
- Source: `project-context/phase2.3/lineage-integrity-review.md` §L-3.

### ADR-038: cycle_id threaded into dispatcher log emissions
- Decision: Every dispatcher log emission (`[DISPATCH_*]`, `[DEPLOY_*]`) carries the reconciler's `cycle=<8-hex>` correlation tag. Sweep / heartbeat / writeHistory emissions are not threaded (no in-cycle provenance to attach).
- Alternatives considered:
  - Add `operations.cycle_id` column for on-disk correlation (deferred to Phase 2.5: schema migration churn).
  - Use slog with structured fields (deferred to Phase 2.5: large refactor).
- Rationale: Log-only propagation closes the cross-component grep gap with zero schema cost. Phase 2.5 will revisit on-disk correlation as part of broader observability work.
- Source: `project-context/phase2.3/observability-integrity.md` §O-1.

---

## Decisions That Have Been Reversed or Superseded

(None yet at the time of file creation. As decisions evolve, superseded entries move here and the current entry references them.)

---

## How To Add a New Decision

When you make a significant decision:

1. Find the next ADR number.
2. Use the template at the top of this file.
3. Be specific about context. Future readers need to understand what situation prompted this.
4. List alternatives that were considered, not just the chosen one.
5. State consequences honestly, including the downsides.
6. Reference related ADRs.

Bad ADR titles: "Use Go", "Add encryption". Good ADR titles: "Go for backend services because of Kubernetes ecosystem", "AES-256-GCM for credential encryption at rest with KMS planned".

---

## What Counts as a "Significant" Decision

You should add an ADR for:
- Choice of programming language, framework, or major library
- Choice of database, message broker, cache, or storage system
- Choice of authentication mechanism
- Choice of API style (REST, GraphQL, gRPC)
- Choice of deployment topology (monolith vs. microservices)
- Choice of an architectural pattern (event sourcing, CQRS, etc.)
- Major data model decisions (denormalization, sharding strategy)
- Security architecture choices
- Adding or removing a major external dependency
- Setting a long-term policy (retention, rate limits, defaults)

You don't need an ADR for:
- Choice of variable name
- Implementation detail of a single function
- Bug fix that doesn't change architecture
- Refactor that doesn't change behavior

When in doubt, add an ADR. The cost of an extra ADR is low. The cost of an undocumented architectural decision is high.

---

## Review Schedule

ADRs are not re-reviewed on a schedule. They are revisited when:
- A new requirement makes the old decision questionable
- A new technology emerges that solves the problem better
- Operational experience reveals the decision was wrong

When revisiting, do not edit the old ADR. Write a new one. Reference the old one. Mark the old one as superseded. The history is preserved.
