# KNOWN_ISSUES.md — Registry of Known Issues, Deferred Work, and Accepted Limitations

> **This file is a living document.** It records every known issue in AutoStack that has been identified but not yet resolved. It also records limitations that are intentional and accepted at this stage. The purpose is to prevent AI agents and engineers from "fixing" things that were deliberately left as-is, and to maintain a clear picture of what is broken, what is deferred, and what is accepted.

---

## How To Read This File

Each entry has the following structure:

```
### ISSUE-NNN: Short title
- Status: [open | deferred | accepted | wont-fix | resolved]
- Severity: [critical | high | medium | low]
- Affects: [list of components]
- Discovered: [date]
- Discovered by: [author]
- Description: [what is the issue]
- Why deferred: [if applicable]
- Workaround: [if applicable]
- Resolution target: [phase/milestone or "none"]
- Notes: [additional context]
```

Statuses defined:
- **open** — actively broken, needs to be fixed
- **deferred** — known issue, fix postponed to a specific phase
- **accepted** — known limitation, not planned to fix in current scope
- **wont-fix** — explicit decision not to fix (with rationale)
- **resolved** — fixed (kept for history)

---

## Critical Issues

These are issues that block production readiness. They must be resolved before AutoStack is offered to external users beyond friendly beta.

### ISSUE-001: Operator runs with cluster-admin RBAC
- Status: open
- Severity: critical
- Affects: Kubernetes operator, deployment installation, security posture
- Description: The Kubernetes operator currently requires `cluster-admin` ClusterRole. This is acceptable for development and personal use but is a non-starter for any enterprise customer. Enterprises will not grant cluster-admin to a third-party operator.
- Why deferred: Required for initial development velocity. Scoped RBAC requires careful resource-by-resource permission design.
- Workaround: For dev/demo only. Production installations require scoped RBAC.
- Resolution target: Phase 2 (Helm chart hardening phase). Must be resolved before public release.
- Notes: The scoped RBAC requires creating a custom ClusterRole that allows: create/update/delete on Deployments, Services, Ingresses, HPAs, PVCs, ConfigMaps, Secrets within namespaces matching a label selector. Plus read on Pods, Events, Metrics. Plus the CRD itself.

### ISSUE-002: PocketBase credential encryption uses environment variable key
- Status: open
- Severity: critical
- Affects: cloud_accounts, registry_credentials, all encrypted fields
- Description: The current encryption-at-rest for cloud credentials uses an AES-256 key loaded from an environment variable (`AUTOSTACK_ENCRYPTION_KEY`). If this environment variable leaks (logs, error messages, container runtime exposure), all stored credentials are compromised.
- Why deferred: Functional encryption is in place. Hardware/KMS-backed key management is a Phase 3 enterprise feature.
- Workaround: Set the environment variable via Kubernetes Secret, not in plaintext config. Restrict access to the deployment manifest. Rotate the key periodically (which requires re-encrypting all existing data).
- Resolution target: Phase 3 (Enterprise hardening). Integrate with AWS KMS, GCP KMS, Azure Key Vault, or HashiCorp Vault for key management.
- Notes: When this is fixed, the encryption key itself becomes a reference (not a value), and decryption requires calling the KMS provider. This affects cold-start latency for any operation that requires decrypting a credential.

### ISSUE-003: No multi-tenant isolation between users
- Status: deferred
- Severity: critical (for enterprise)
- Affects: every collection, every API endpoint, every WebSocket connection
- Description: AutoStack currently operates in single-user mode. Each user has their own projects, deployments, blueprints. There is no concept of organizations, teams, or shared workspaces. For any company with more than one developer, this is a hard blocker.
- Why deferred: Single-user is sufficient for early validation. Multi-tenancy is a major refactor and is planned for Phase 2.
- Workaround: For demos and small teams, use shared credentials (not recommended for production).
- Resolution target: Phase 2 (Organizations and RBAC).
- Notes: The migration to multi-tenancy adds `workspace_id` to every relevant collection, adds a `workspaces` collection, adds a `workspace_members` join collection, and adds workspace-scoped query filters everywhere. Authentication will be extended to support workspace selection at login.

### ISSUE-004: No audit log for sensitive operations
- Status: open
- Severity: critical (for SOC2)
- Affects: compliance posture, incident investigation, customer trust
- Description: There is no immutable audit log capturing who did what when. Deployments are created without audit. Credentials are accessed without audit. Configuration changes have no trail.
- Why deferred: Functional features came first. Audit logging is being designed in conjunction with the multi-tenant migration.
- Workaround: PocketBase request logs capture some information at the HTTP level, but they are not designed for audit purposes and do not cover internal operations.
- Resolution target: Phase 2 (alongside organizations).
- Notes: The audit log collection must be append-only (no updates, no deletes). It must capture: actor, action, resource type, resource id, timestamp, IP address, user agent, request id, before/after state diff (for changes), result (success/failure).

### ISSUE-005: No SOC2/ISO27001/HIPAA compliance attestation
- Status: accepted
- Severity: critical (for enterprise sales)
- Affects: enterprise customer acquisition
- Description: AutoStack has no formal compliance certifications. This is acceptable for individual developers and small teams but blocks enterprise sales conversations.
- Why deferred: Compliance is a multi-month process requiring auditor engagement and significant operational maturity. Cannot be done before the platform is stable.
- Workaround: For now, target customer segments that do not require compliance certifications. Provide documentation of security controls for customers conducting their own assessments.
- Resolution target: Phase 4 or later. Pursue SOC2 Type II first.
- Notes: Compliance work begins after the security model (Phase 2) is complete and operational practices (logging, monitoring, incident response) are mature.

---

## High-Severity Issues

These are issues that materially affect users but do not block initial release.

### ISSUE-006: No drift detection for cloud deployments
- Status: open
- Severity: high
- Affects: cloud reconciliation, user trust
- Description: For cloud deployments, the system does not yet detect when the actual cloud resources have diverged from the desired state stored in PocketBase. A user could modify an ECS service in the AWS console and AutoStack would not know.
- Why deferred: The Provider interface is being designed. Drift detection is a Phase 1 feature but is being implemented after the first cloud provider integration is complete.
- Workaround: Educate users not to modify cloud resources outside AutoStack. Document this limitation prominently in the cloud onboarding UI.
- Resolution target: Phase 1, immediately after first cloud provider integration is functional.
- Notes: Drift detection requires a periodic reconciliation cycle that compares desired state to actual state. The reconciliation interval is 30 seconds by default. Drift remediation (auto-fixing drift) is a separate feature and is opt-in per deployment.

### ISSUE-007: Cost estimates may diverge significantly from actual bills
- Status: accepted
- Severity: high
- Affects: user trust, cost visibility
- Description: Cost estimates currently account for compute (vCPU, memory) and basic infrastructure (load balancer, NAT gateway when applicable). They do not include: data transfer egress, CloudWatch logs ingestion, ECR image storage and pulls, ALB request charges, NAT gateway data processing charges. Actual bills may be 1.5x to 3x the estimate for high-traffic workloads.
- Why accepted: Perfect cost prediction is impossible without modeling each user's traffic pattern, which we cannot predict. We must be honest about the limits of estimation.
- Workaround: Display estimates as ranges with explicit assumptions. Show actual cost from cloud billing as soon as it is available. Implement cost anomaly detection (alert when actual cost exceeds estimate by 2x).
- Resolution target: Phase 2 (improved cost intelligence with Infracost integration).
- Notes: The estimate must always be displayed alongside the assumption it is based on. Never display a single number. Always display a range with clear caveats.

### ISSUE-008: No DNS or domain management built in
- Status: deferred
- Severity: high
- Affects: production readiness of cloud deployments
- Description: When a user deploys to ECS Fargate, they get an ALB DNS name (something like `abc123.us-east-1.elb.amazonaws.com`). This is not production-ready. Production deployments need custom domains, SSL certificates, and DNS records. AutoStack does not automate any of this.
- Why deferred: Domain management is a significant feature requiring DNS provider integrations, certificate provisioning (ACM, Let's Encrypt), and verification flows. It is planned for after cloud deployment is solid.
- Workaround: User manually configures their DNS provider to point at the cloud-provided endpoint. User manually provisions a certificate.
- Resolution target: Phase 2 (production polish).
- Notes: The DNS feature requires integrations with Route53, Cloudflare, and at least one other DNS provider. The certificate feature requires ACM for AWS, Let's Encrypt for other cases. The verification flow requires either DNS access or domain ownership verification.

### ISSUE-009: No support for stateful workloads (StatefulSets)
- Status: accepted
- Severity: high
- Affects: database deployments, certain user use cases
- Description: AutoStack only supports stateless deployments. Users wanting to deploy databases, Redis, or other stateful workloads cannot do so. The Rollout CRD only generates Kubernetes Deployments, not StatefulSets.
- Why accepted: Stateful workloads are operationally complex (persistent storage management, backup, restoration, failure handling). Best practice is to use managed database services (RDS, Cloud SQL, MongoDB Atlas) for state.
- Workaround: Document that AutoStack is for stateless workloads only. Provide guidance on using managed database services. Allow connection to external managed databases via secret references.
- Resolution target: Not planned. Wont-fix unless strong demand emerges.
- Notes: If we ever add stateful workload support, it would be a major feature involving StatefulSet CRD support, volume snapshot management, backup integration. The scope of work is significant and the user demand has not been validated.

### ISSUE-010: Blueprint versioning not implemented
- Status: deferred
- Severity: high
- Affects: blueprint marketplace, breaking change management
- Description: Blueprints are currently versionless. If the maintainer of a blueprint changes it, every user of the blueprint sees the new version on next use. There is no way to pin to a specific version, view the version history, or know what changed.
- Why deferred: Initial blueprint system focused on creation and sharing. Versioning is a Phase 2 feature.
- Workaround: Users can fork a blueprint to lock its version. Maintainers should communicate breaking changes through release notes.
- Resolution target: Phase 2.
- Notes: Blueprint versioning requires: a `blueprint_versions` collection, version numbering scheme (semver-style), changelog field per version, ability to pin a deployment to a specific blueprint version, deprecation flags on versions.

### ISSUE-011: No deployment dependency ordering
- Status: deferred
- Severity: high
- Affects: multi-service application deployment
- Description: Deployments are deployed independently. There is no way to say "deploy Redis first, wait for it to be healthy, then deploy the application that depends on Redis." Users with multi-service applications must coordinate deployment ordering manually.
- Why deferred: Complex feature requiring dependency graph evaluation, health check coordination, rollback ordering. Planned for after single-deployment flow is solid.
- Workaround: Deploy dependencies first, manually verify they are healthy, then deploy dependents. Document this pattern.
- Resolution target: Phase 3 (advanced workflows).
- Notes: Implementation requires extending the deployment model to support dependency references, a workflow engine for ordered deployment, and rollback ordering (reverse dependency order). May leverage GitOps patterns when those are added.

### ISSUE-012: No log search or aggregation
- Status: deferred
- Severity: high
- Affects: debugging, incident investigation, compliance
- Description: Logs stream live to the frontend via WebSocket, but they are not stored for historical search. A user cannot query "show me all errors in the last 24 hours across all my services." Logs are ephemeral once the WebSocket disconnects.
- Why deferred: Log aggregation is a major feature involving storage backend choice, query interface, retention policies. Cloud-native logging integration is the planned path.
- Workaround: Users use the cloud provider's native logging (CloudWatch, Cloud Logging, Azure Monitor) for historical queries. For Kubernetes, users can install their own logging stack.
- Resolution target: Phase 3.
- Notes: The plan is to integrate with Loki for self-hosted users and expose cloud-native log APIs (CloudWatch Insights, Cloud Logging) for cloud deployments. No plans to build a proprietary log storage system.

---

## Medium-Severity Issues

These are issues that should be addressed but do not significantly impact usability.

### ISSUE-013: No cold-start awareness for serverless cloud targets
- Status: open
- Severity: medium
- Affects: Cloud Run, Azure Container Apps deployments
- Description: When deploying to Cloud Run or ACA with scale-to-zero enabled, users do not see any warning about cold start latency. A user accustomed to Kubernetes (where pods are always running) may be surprised by the first-request latency after idle periods.
- Why deferred: Cloud Run integration is being implemented. Cold start warnings are a polish item.
- Workaround: Educate users in onboarding. Display cold start expectations in the deployment configuration UI when scale-to-zero is enabled.
- Resolution target: Phase 1 (alongside Cloud Run integration).
- Notes: The UI should show "this deployment may take 2-5 seconds to respond after periods of inactivity due to cold start" when scale-to-zero is enabled. There should be an option to set minimum instances to 1 to avoid cold starts (with cost implications shown).

### ISSUE-014: No health check translation between Kubernetes and cloud
- Status: open
- Severity: medium
- Affects: cross-target blueprint usage
- Description: A blueprint with Kubernetes-style liveness and readiness probes does not automatically translate to ECS health checks or Cloud Run startup probes. The mapping is provider-specific and currently must be done manually.
- Why deferred: Translation logic requires careful per-provider implementation.
- Workaround: Blueprints declare their target compatibility. Blueprints targeting cloud have cloud-style health checks. Blueprints for Kubernetes have Kubernetes probes.
- Resolution target: Phase 2.
- Notes: The right abstraction is to expose health check intent (a path + port + expected response) and translate per-provider. Kubernetes gets liveness/readiness probes. ECS gets ALB target group health check. Cloud Run gets startup probe configuration.

### ISSUE-015: No VPC management for AWS ECS deployments
- Status: open
- Severity: medium
- Affects: AWS deployments
- Description: For AWS ECS Fargate, the system requires the user to have a VPC. AutoStack does not create VPCs, subnets, security groups, or NAT gateways. The user must either use the default VPC (insecure for enterprise) or configure their own VPC manually.
- Why deferred: VPC creation is significant infrastructure work that touches many AWS services.
- Workaround: User provides VPC ID and subnet IDs during cloud account setup. Document the required networking configuration.
- Resolution target: Phase 2 (VPC management features).
- Notes: When implemented, AutoStack should be able to create a managed VPC for a user on request, with sensible defaults (private subnets for ECS tasks, public subnets for ALB, NAT gateway for outbound traffic). This is a significant feature.

### ISSUE-016: WebSocket connections do not survive backend restarts
- Status: open
- Severity: medium
- Affects: user experience during backend deployments
- Description: When the backend service is restarted (e.g., during a deployment of AutoStack itself), all WebSocket connections are dropped. The frontend has reconnection logic, but there is a window where the user sees disconnected status.
- Why accepted: This is standard behavior for WebSocket servers. The reconnection logic mitigates the impact.
- Workaround: The frontend reconnects automatically. The user sees a brief "reconnecting" state.
- Resolution target: Not planned to fully fix. Future work may add WebSocket connection migration (sticky sessions) when running multiple backend replicas.
- Notes: Mitigated by: rolling deployments (one backend replica restarts at a time), frontend auto-reconnect, server-sent connection lifecycle messages.

### ISSUE-017: No rate limiting on AI feature usage
- Status: open
- Severity: medium
- Affects: AI features (incident explainer, right-sizer, Compose converter)
- Description: AI features call out to the user's configured AI provider (Anthropic or OpenAI). There is currently no server-side rate limiting on these calls. A bug or malicious user could cause a large number of AI calls, racking up costs on the user's AI provider account.
- Why deferred: AI features are early. Rate limiting is being designed.
- Workaround: User-provided API keys are scoped to the user's own account. The user, not AutoStack, bears the cost.
- Resolution target: Phase 1, before AI features are enabled for general users.
- Notes: Rate limiting should be: per-user, per-feature, with configurable thresholds. Should default to conservative limits (e.g., 10 incident explanations per hour per user).

### ISSUE-018: Reconciliation service has no circuit breaker for cloud API errors
- Status: open
- Severity: medium
- Affects: cloud reconciliation under provider outages
- Description: If a cloud provider's API is degraded or down, the reconciliation service continues to retry on each cycle. It does not implement circuit breaker pattern that would stop retrying after sustained failure.
- Why deferred: Initial implementation focused on the happy path.
- Workaround: Manual intervention if a cloud provider has prolonged outages.
- Resolution target: Phase 1 (reconciliation hardening).
- Notes: Circuit breaker implementation should: track failure rate per provider per region, open the circuit after threshold failures, periodically attempt a probe to test recovery, close the circuit after successful probes.

### ISSUE-019: No background cleanup of orphaned cloud resources
- Status: open
- Severity: medium
- Affects: cost, cloud account hygiene
- Description: If a deployment is deleted in AutoStack but the cloud cleanup fails (network error, permission issue), the cloud resources are orphaned. There is no background process that detects and cleans up these orphans.
- Why deferred: The single-deployment cleanup path works. Orphan detection requires periodic scanning of cloud resources by tag.
- Workaround: User must manually clean up if a deletion failed. Audit log can be used to find what should have been deleted.
- Resolution target: Phase 2.
- Notes: The cleanup job should: list all cloud resources tagged with `autostack:managed-by=autostack`, check each against PocketBase records, delete resources that have no corresponding record. Must be conservative — only delete tagged resources that AutoStack created.

### ISSUE-020: Frontend does not handle slow cloud operations gracefully
- Status: open
- Severity: medium
- Affects: user experience during initial cloud deployments
- Description: Some cloud operations (especially VPC creation, ALB provisioning) take 2-5 minutes. The frontend currently shows generic "deploying" status without indicating progress. Users may think the system is broken.
- Why deferred: Initial deployment flow focused on Kubernetes (which is faster).
- Workaround: Display estimated time during deployment. Show specific status messages from the provider when available.
- Resolution target: Phase 1 (cloud UX polish).
- Notes: The frontend should show: current step being executed (e.g., "creating VPC", "creating security groups", "creating ECS service"), elapsed time, expected total time based on past deployments to the same provider.

---

## Low-Severity Issues

These are issues that are known but have minimal user impact.

### ISSUE-021: PocketBase migrations are JavaScript files, not Go
- Status: accepted
- Severity: low
- Affects: developer experience, migration testing
- Description: PocketBase uses JavaScript for migration files. The rest of the backend is Go. This creates a context switch for developers writing migrations.
- Why accepted: This is a PocketBase convention. Working around it would create more complexity than it solves.
- Workaround: Document the migration writing process. Provide examples.
- Resolution target: Wont-fix.
- Notes: If we ever migrate from PocketBase to PostgreSQL, the migration tool would change with it.

### ISSUE-022: No internationalization (i18n) of the frontend
- Status: accepted
- Severity: low
- Affects: non-English-speaking users
- Description: The frontend is English-only. There is no i18n framework, no translation files, no locale switching.
- Why accepted: Out of scope for initial release. Adding i18n later requires reworking the UI strings.
- Workaround: None. English-only for now.
- Resolution target: Not planned for current scope.
- Notes: If pursued later, this is a significant refactor. Every UI string becomes a translation key. Right-to-left languages add layout complexity.

### ISSUE-023: No dark mode in the frontend
- Status: accepted
- Severity: low
- Affects: user preference
- Description: The frontend has one theme (light). Many developer tools offer dark mode. AutoStack does not.
- Why accepted: Cosmetic. Not blocking.
- Workaround: None.
- Resolution target: Phase 4 or later (polish phase).
- Notes: Dark mode requires a theme system, CSS variable refactor, user preference storage.

### ISSUE-024: Frontend file uploads are limited to 5MB
- Status: accepted
- Severity: low
- Affects: blueprint uploads, configuration imports
- Description: The frontend has a hardcoded 5MB upload limit. This is a guard against accidental large uploads.
- Why accepted: 5MB is generous for any reasonable configuration file. Real use cases stay well below this.
- Workaround: For larger files, use the API directly with appropriate chunking.
- Resolution target: Adjust if a real use case emerges.
- Notes: The backend can accept larger uploads. The 5MB limit is just on the frontend for UX safety.

### ISSUE-025: No GraphQL API
- Status: wont-fix
- Severity: low
- Affects: client API ergonomics
- Description: The API is REST + WebSocket. Some developers prefer GraphQL for flexible querying.
- Why wont-fix: REST + WebSocket is sufficient for the use cases. Adding GraphQL doubles the API surface.
- Workaround: Use the REST API. The endpoints are well-documented.
- Resolution target: Not planned.
- Notes: If a strong case emerges later, we can revisit. For now, REST is the contract.

---

## Refactor Candidates

These are areas of the code that work but are known to need refactoring at some point.

### ISSUE-026: Cost calculation logic spread across multiple files
- Status: deferred
- Affects: pkg/cost, pkg/providers
- Description: Cost calculation logic is currently fragmented. Some logic is in the provider implementations, some is in `pkg/cost`. This should be consolidated into a single cost calculation service.
- Why deferred: Refactor for refactor's sake. Code works as-is.
- Resolution target: After Phase 1 is stable.

### ISSUE-027: Frontend state management is ad-hoc
- Status: deferred
- Affects: frontend/src/lib/stores
- Description: Frontend state is managed through individual Svelte stores. There is no centralized state architecture. As the application grows, this will become harder to reason about.
- Why deferred: Current state model is sufficient. A premature state refactor would slow down feature work.
- Resolution target: Phase 3 (frontend polish).

### ISSUE-028: Provider interface may need expansion for advanced features
- Status: noted
- Affects: pkg/providers
- Description: The current Provider interface covers basic deployment lifecycle. Advanced features (canary deployments, blue/green, traffic splitting) will require interface expansion.
- Why noted: The interface is correct for current scope. It will need extension for Phase 3 features.
- Resolution target: Phase 3.

### ISSUE-029: WebSocket message schemas not formally defined
- Status: open
- Affects: frontend/backend coordination
- Description: WebSocket messages are JSON with implicit schemas. There is no formal schema definition (e.g., JSON Schema, Protobuf). This means changes can silently break the frontend.
- Why deferred: Implicit contract works. Formal contracts are a Phase 2 polish item.
- Resolution target: Phase 2.
- Notes: When formalized, generate frontend TypeScript types from the same schema definition.

### ISSUE-030: Background job framework is custom
- Status: noted
- Affects: pkg/reconciler, pkg/cost (background recalc), other background work
- Description: Background jobs (reconciliation, auto-update polling, cost recalculation) use custom goroutine management. There is no shared job framework with retry, scheduling, observability.
- Why noted: Custom is fine for the current job count. As the number of background jobs grows, a shared framework becomes valuable.
- Resolution target: Phase 3.

---

## Issues That Affect Specific Users / Deployments

### ISSUE-031: Users in regions without low-latency AWS access experience slow cloud account validation
- Status: noted
- Affects: users in some geographic regions
- Description: AWS API calls from the backend to validate cloud account credentials can take 5-10 seconds in regions far from the chosen AWS region. The UI shows a spinner during this time.
- Why noted: This is intrinsic to the AWS API latency. We cannot make it faster.
- Workaround: Display the expected wait time. Show a more informative spinner.
- Resolution target: Phase 1 (UX polish).

### ISSUE-032: Users with very large numbers of deployments (>100) experience slow dashboard loading
- Status: open
- Affects: power users
- Description: The dashboard loads all deployments for the user in a single query. With more than 100 deployments, this becomes slow.
- Why deferred: Few users have hit this scale yet.
- Workaround: Use the search/filter to narrow down displayed deployments.
- Resolution target: Phase 2 (pagination, lazy loading).

### ISSUE-033: Users with custom CA certificates for private registries have configuration friction
- Status: open
- Affects: enterprise users with private registries on internal infrastructure
- Description: Configuring a custom CA certificate for a private registry requires uploading the cert as an environment variable in the deployment manifest. The UI does not have a dedicated field for this.
- Why deferred: Edge case. Real demand is small.
- Workaround: Use the environment variable approach. Document it.
- Resolution target: When demand justifies UI investment.

---

## Issues Discovered During Development (Resolved or in Progress)

This section is for tracking issues found during active development that are being worked on. Entries graduate to "resolved" or get moved to one of the other sections.

(No active entries at time of file creation. Add entries here as they arise.)

---

## Process Issues (Not Code)

These are issues with the development process or documentation, not with the code itself.

### ISSUE-034: No automated documentation deployment
- Status: open
- Severity: low
- Affects: documentation discoverability
- Description: Documentation lives in the repository. There is no auto-generated documentation site, no published API reference.
- Why deferred: Documentation is read by AI agents and engineers directly from the repository. A published site is a future polish item.
- Workaround: Read documentation directly in the repository.
- Resolution target: Phase 3 (developer experience).

### ISSUE-035: No public CI/CD pipeline
- Status: open
- Severity: medium
- Affects: contributor experience, release reliability
- Description: There is no published CI pipeline visible to contributors. Building releases is currently manual.
- Why deferred: Release frequency is low. CI investment is planned alongside open-source release.
- Resolution target: Phase 2 (alongside open-source release).

### ISSUE-036: No formal contributor guide
- Status: open
- Severity: low
- Affects: contributor experience
- Description: There is no CONTRIBUTING.md, no code-of-conduct, no formal review process documented for external contributors.
- Why deferred: External contribution is not yet open. When it opens, this becomes priority.
- Resolution target: Before open-source release.

---

## Accepted Limitations (Will Not Fix)

These are intentional design choices that may look like issues but are deliberate.

### LIMITATION-001: One cluster per project
- Affects: multi-cluster scenarios
- Rationale: Projects map cleanly to namespaces in Kubernetes. Multi-cluster is a different concept (federation). It is being added separately as a Phase 3 feature, not by changing the project model.

### LIMITATION-002: No on-premise non-Kubernetes deployment targets
- Affects: bare-metal Docker deployments, VMs
- Rationale: AutoStack is for Kubernetes and managed container services. Bare-metal Docker is out of scope. Customers wanting bare-metal can wrap their setup in Kubernetes.

### LIMITATION-003: No support for non-container workloads
- Affects: VM workloads, serverless function deployments (Lambda, Cloud Functions)
- Rationale: AutoStack is container-focused. Serverless function deployment is a different model and is out of scope.

### LIMITATION-004: No built-in CI/CD pipeline for user applications
- Affects: users wanting full build + deploy from source
- Rationale: AutoStack deploys pre-built images. Building images is delegated to existing CI tools (GitHub Actions, GitLab CI, etc.). We provide webhook integration to trigger deploys after a build.

### LIMITATION-005: English-only documentation
- Affects: non-English users
- Rationale: Out of scope for current release. May change if there is significant demand.

### LIMITATION-006: No mobile-optimized UI
- Affects: mobile users
- Rationale: AutoStack is a developer tool. Most users will use it on desktop. Mobile responsiveness is best-effort but not a design priority.

---

## How To Add a New Issue

Use the next available `ISSUE-NNN` number. Follow the template at the top of this file. Set the status accurately. Be specific about who is affected and how.

When you fix an issue, change its status to `resolved` and add a note about the resolution. Do not delete entries. The history is valuable.

When you decide not to fix an issue, change its status to `wont-fix` and document the rationale. Future contributors will want to know why.

---

## Maintenance

This file is reviewed at the start of every milestone planning session. Issues are evaluated against the milestone scope. Resolution targets are adjusted as needed. Issues that have been resolved are summarized in the release notes.

Issues should be added in real-time as they are discovered. Do not batch up issue reports. The act of writing them down captures information that would otherwise be lost.
