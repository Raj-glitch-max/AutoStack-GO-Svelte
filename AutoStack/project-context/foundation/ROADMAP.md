# ROADMAP.md — AutoStack Implementation Roadmap

---

## Guiding Principle

Each phase must be **fully complete, tested, and production-stable** before the next phase begins. No phase is started while the previous phase has open critical issues. Features are not partially shipped.

The North Star remains: **a developer with a Docker image deploys to production in under 10 minutes.**

---

## Current State (Pre-Roadmap Baseline)

**COMPLETE AND WORKING** (do not modify unless explicitly in a phase task):
- Kubernetes CRD-based deployment management
- PocketBase auth and data layer
- WebSocket real-time pod/service/ingress/PVC/job event streaming
- Live log streaming from pods
- Rollout history, diff view, one-click rollback
- Auto-update scheduler (semver + timestamp policies)
- Blueprint sharing system
- Project → namespace mapping
- SvelteKit frontend with deployment management UI
- Registry credential management

**Total codebase**: ~332 files, working production-grade Kubernetes system

---

## Phase 0 — Foundation and Security Hardening

**Objective**: Harden the existing Kubernetes system to production-enterprise standard. Establish the data model foundation for all subsequent cloud work. No new features visible to users.

**Duration estimate**: 3-4 weeks

**Tasks**:

### 0.1 — Helm Chart for Kubernetes Operator
- [ ] Create Helm chart in `/helm/` directory
- [ ] Implement least-privilege RBAC (remove cluster-admin)
- [ ] Configurable namespace, resource limits, image registry
- [ ] Air-gapped installation support (configurable image registry)
- [ ] Operator upgrade path documented (CRD version compatibility)
- [ ] Helm chart values documented in `helm/README.md`

### 0.2 — PocketBase Schema: Organization Model
- [ ] Add `organizations` collection
- [ ] Add `workspaces` collection
- [ ] Add `workspace_members` collection
- [ ] Migration: existing `users` get a personal organization + workspace (1:1 for existing users)
- [ ] Migration: existing `projects` get a `workspace_id` (nullable → added as non-breaking)
- [ ] No UI changes yet — schema only

### 0.3 — Audit Log Implementation
- [ ] Create `audit_log` collection (append-only)
- [ ] Implement audit log middleware in Go backend — wraps all write operations
- [ ] Audit log writes for: rollout CRUD, project CRUD, login events, API key operations
- [ ] Credential sanitization: ensure no encrypted fields appear in audit records
- [ ] Audit log read API: `/api/v1/audit?workspace_id=&from=&to=`
- [ ] Test: confirm audit log cannot be deleted or updated via any API path

### 0.4 — Secrets Separation
- [ ] Migrate deployment secrets out of rollout spec JSON into separate reference model
- [ ] For Kubernetes: secrets stored as Kubernetes Secrets, only references in rollout spec
- [ ] Update rollout spec schema to use `secretRefs` instead of inline values
- [ ] Update UI: secret entry creates the K8s secret, UI shows reference only
- [ ] Existing rollouts migrated to new schema without breaking deployments

### 0.5 — Credential Encryption Validation
- [ ] Audit all PocketBase fields — confirm no credentials in plaintext
- [ ] Implement logging middleware credential redaction (see SECURITY_AND_ACCESS.md)
- [ ] Add test: scan all log output in test suite for known credential patterns
- [ ] Document `AUTOSTACK_ENCRYPTION_KEY` generation and rotation procedure

### 0.6 — API Keys Infrastructure
- [ ] Create `api_keys` collection
- [ ] API key generation endpoint: `POST /api/v1/auth/api-keys`
- [ ] API key authentication middleware
- [ ] Scope enforcement in authorization layer
- [ ] API key revocation: `DELETE /api/v1/auth/api-keys/:id`
- [ ] UI: API key management page in settings

### 0.7 — Multi-Cluster Kubernetes Support
- [ ] Data model: `projects.cluster_config` supports multiple clusters (one per project, but different projects can use different clusters)
- [ ] Add cluster connection management UI: connect multiple clusters
- [ ] Kubernetes client factory: create client per cluster config, not a global singleton
- [ ] Watcher pool: per-cluster watchers, not assuming a single in-cluster config
- [ ] Test: two clusters, two projects, deployments on each, verify independent management

**Phase 0 Completion Criteria**:
- Helm chart installs successfully on a vanilla Kubernetes cluster with no cluster-admin
- All write operations produce audit log entries
- No credential values visible in any log output at any log level
- API keys authenticate correctly and scope enforcement works
- All existing Kubernetes functionality continues working unchanged

---

## Phase 1 — Cloud Provider Foundation

**Objective**: Add the Provider abstraction layer and implement Google Cloud Run as the first cloud target. Full end-to-end working: connect account → deploy → monitor → rollback → destroy.

**Dependency**: Phase 0 complete

**Duration estimate**: 4-6 weeks

**Why Cloud Run First**: Fewest networking decisions (no VPC required for public services), cheapest to test, scale-to-zero saves cost during development, traffic splitting is native (enables progressive delivery demo), most forgiving API.

### 1.1 — Provider Interface
- [ ] Define `Provider` interface in `/pkg/providers/interface.go`
- [ ] Define `DeploySpec` struct (provider-agnostic deployment description)
- [ ] Define `TargetStatus`, `TargetMetrics`, `CostEstimate`, `ActualCost` return types
- [ ] Define `QuotaCheck` struct for pre-deployment quota validation
- [ ] Add `KubernetesProvider` stub that wraps existing K8s path (for interface completeness)
- [ ] Provider registry: map of provider type → Provider implementation

### 1.2 — Cloud Account Management
- [ ] `cloud_accounts` collection (see DATA_MODEL.md)
- [ ] Cloud account CRUD API
- [ ] Credential encryption/decryption service
- [ ] Validate endpoint: `POST /api/v1/cloud-accounts/:id/validate`
- [ ] UI: Add Cloud Account flow with step-by-step IAM setup instructions
- [ ] UI: Permission checklist — what AutoStack needs, why, how to grant it
- [ ] UI: "Validate Connection" button with specific, actionable error messages
- [ ] Test: validate with correct credentials → success; validate with missing permissions → specific error message naming the missing permission

### 1.3 — Google Cloud Run Provider
- [ ] Implement `GCPCloudRunProvider` in `/pkg/providers/cloudrun/`
- [ ] `ValidateCredentials`: test service account permissions for Cloud Run + Artifact Registry
- [ ] `Deploy`: create or update Cloud Run service from DeploySpec
- [ ] `GetStatus`: poll service status, map to normalized TargetStatus
- [ ] `GetMetrics`: poll Cloud Monitoring for CPU/memory metrics
- [ ] `StreamLogs`: stream Cloud Logging entries to io.Writer
- [ ] `EstimateCost`: call GCP Pricing API for vCPU + memory rates
- [ ] `GetActualCost`: read from GCP Billing API (or null if billing API not enabled)
- [ ] `Destroy`: delete Cloud Run service and associated resources
- [ ] `ListRegions`: return available Cloud Run regions
- [ ] `CheckQuotas`: verify Cloud Run quota availability

### 1.4 — Cloud Reconciler
- [ ] Implement reconciliation loop in `/pkg/reconciler/`
- [ ] Poll PocketBase for all `deployment_targets` with status != deleted
- [ ] For each target: call Provider.GetStatus() and compare with desired state
- [ ] Update PocketBase status fields after polling
- [ ] Detect drift: log and set `drift_detected=true` when actual ≠ desired
- [ ] Synthesize WebSocket events from polling results (matching frontend's expected message format)
- [ ] Distributed lock: use PocketBase record to ensure single-instance reconciliation
- [ ] Test: deploy to Cloud Run, manually change something in GCP console, verify drift is detected within 60 seconds

### 1.5 — Network Config Management
- [ ] `network_configs` collection (for future AWS VPC management — GCP doesn't need this for Cloud Run)
- [ ] For Cloud Run: record the project ID and region as the "network config"

### 1.6 — Cost Estimation
- [ ] `cost_estimates` collection
- [ ] Cost estimation service in `/pkg/cost/`
- [ ] GCP Pricing API client (free, no auth required for public pricing)
- [ ] Infracost API client (for infrastructure overhead)
- [ ] Multi-provider comparison: calculate estimate for all connected providers simultaneously
- [ ] API: `POST /api/v1/cost/estimate` with DeploySpec → returns comparison across connected providers
- [ ] UI: Pre-deployment cost comparison view
- [ ] Show: low estimate, high estimate, uncertainty note
- [ ] Never show a single definitive number without a range

### 1.7 — Resource Tagging and Cleanup
- [ ] All GCP resources created by AutoStack tagged with: `autostack-managed=true`, `autostack-deployment-id=<id>`, `autostack-workspace=<id>`, `autostack-project=<id>`
- [ ] Destroy workflow: when rollout is deleted, trigger Provider.Destroy() before marking deleted
- [ ] Destroy failure handling: if Destroy fails, deployment stays in `deleting` status with error shown
- [ ] Orphan detection: weekly scan of GCP projects for autostack-tagged resources with no PocketBase deployment record
- [ ] Force-delete option: removes PocketBase record with warning that GCP resources remain

### 1.8 — DNS and Domain Management (Cloud Run)
- [ ] `dns_records` collection
- [ ] Cloud Run custom domain mapping API
- [ ] Domain verification flow: generate verification token → user adds TXT record → platform verifies → enable domain
- [ ] SSL certificate: Cloud Run manages this automatically (display cert status in UI)
- [ ] UI: Custom domain setup flow with DNS record instructions

### 1.9 — Frontend: Cloud Deployment Experience
- [ ] Cloud account selector in new deployment form
- [ ] Provider-specific configuration options (Cloud Run: min instances, CPU throttling, execution environment)
- [ ] Cloud status indicator (distinct from Kubernetes status — shows "cloud provider" badge)
- [ ] Cost estimate display in deployment form before submitting
- [ ] Drift alert banner on deployment detail page when drift is detected
- [ ] Cloud log streaming in same log viewer as Kubernetes logs

**Phase 1 Completion Criteria**:
- A fresh GCP project can be connected and validated in under 5 minutes with clear per-permission error feedback
- A deployment on Cloud Run goes from "deploy clicked" to "live URL shown" without user understanding GCP
- Real-time cost estimate appears before any deployment
- Drift is detected and surfaced to the user within 60 seconds
- Destroy on Cloud Run actually removes all GCP resources (verified by checking GCP console)
- Rollback works: deploys a previous revision on Cloud Run

---

## Phase 2 — AWS ECS Fargate

**Objective**: Add AWS ECS Fargate as the second cloud provider. Most-requested enterprise target.

**Dependency**: Phase 1 complete and stable

**Duration estimate**: 5-7 weeks (longer due to VPC complexity)

### 2.1 — AWS ECS Fargate Provider
- [ ] Implement `AWSECSProvider` in `/pkg/providers/ecs/`
- [ ] All Provider interface methods implemented
- [ ] ECS: task definition management, service creation/update
- [ ] ALB: Application Load Balancer creation and management
- [ ] VPC management: create default AutoStack VPC + subnets + security groups (or use existing)
- [ ] CloudWatch Logs: log group creation, streaming
- [ ] ECR: image pull authentication setup

### 2.2 — VPC/Network Configuration Management
- [ ] `network_configs` collection implemented for AWS
- [ ] First-time setup: create AutoStack VPC in the user's account (or let them select existing)
- [ ] Security group management: per-deployment security groups with minimal required rules
- [ ] UI: network configuration step in AWS deployment form
- [ ] Cost impact: show VPC/NAT gateway cost contribution to estimate

### 2.3 — AWS Pricing and Cost
- [ ] AWS Pricing API client (no auth required for public pricing)
- [ ] Fargate vCPU and memory per-hour rates per region
- [ ] ALB per-hour and per-LCU rates
- [ ] CloudWatch Logs ingestion rates
- [ ] Data transfer rates (egress from AWS)
- [ ] Infracost API for infrastructure overhead
- [ ] Cost estimate: show compute + infrastructure breakdown with uncertainty

### 2.4 — IAM Permission Documentation and Automation
- [ ] Generate per-account IAM policy document for AutoStack's required permissions
- [ ] One-click setup instructions in UI: step-by-step IAM user/role creation
- [ ] Permission validation: test each required permission individually and report which are missing
- [ ] Role assumption support: advanced users can use IAM role assumption instead of static keys

### 2.5 — Cost Sync for AWS
- [ ] AWS Cost Explorer API client
- [ ] Daily cost pull per cloud account, attributed to deployments via tags
- [ ] `cost_records` populated for AWS deployments
- [ ] Cost anomaly detection and notification

**Phase 2 Completion Criteria**:
- AWS ECS Fargate deployment end-to-end working including VPC, ALB, and domain
- Cost estimate within ±25% of actual AWS bill for compute-only workloads
- Permission error messages name the specific missing IAM permission
- All AWS resources created are tagged for attribution

---

## Phase 3 — Organization Model, RBAC, and Enterprise Features

**Objective**: Enable multi-team usage. Make AutoStack ready for enterprise sales.

**Dependency**: Phase 1 or 2 complete (organization model is provider-independent)

**Duration estimate**: 4-5 weeks

### 3.1 — Organizations and Workspaces UI
- [ ] Organization creation and settings
- [ ] Workspace creation, member invitation
- [ ] Workspace-level role assignment
- [ ] Resource quota UI (max deployments, max monthly cost)
- [ ] Cost approval workflow (deploy requires approval if estimate exceeds threshold)

### 3.2 — SSO Integration
- [ ] WorkOS integration or equivalent for SAML 2.0 / OIDC
- [ ] Organization SSO configuration UI (admin only)
- [ ] SCIM user provisioning support (automatic user creation/deprovisioning from identity provider)
- [ ] Existing PocketBase OAuth2 users migrated without disruption

### 3.3 — Audit Log UI
- [ ] Audit log viewer in organization settings
- [ ] Filtering by user, action type, resource type, date range
- [ ] Audit log export (JSON, CSV)
- [ ] Retention configuration per organization

### 3.4 — Notification System
- [ ] Novu integration (or self-hosted)
- [ ] Notification preferences UI per user
- [ ] Workspace-level notification configuration (Slack webhook, Teams webhook, email)
- [ ] Notification triggers: deploy success/fail, rollback, cost anomaly, drift, certificate expiry

### 3.5 — API First Completion
- [ ] Every UI operation has a corresponding API endpoint
- [ ] API documentation generated and published
- [ ] CLI v1 release: `autostack deploy`, `autostack rollback`, `autostack status`, `autostack logs`

**Phase 3 Completion Criteria**:
- Enterprise prospect can demo multi-team workspace separation
- SSO login works with at least one major identity provider (Okta)
- API key authentication works for full CI/CD pipeline (GitHub Actions template provided)
- CLI deploys successfully

---

## Phase 4 — AI Features

**Objective**: Add AI features grounded in real operational data. No AI theater.

**Dependency**: Phase 1+ complete (real incidents and real logs required)

**Duration estimate**: 2-3 weeks

### 4.1 — Incident Explainer
- [ ] `incidents` collection
- [ ] Incident auto-detection: error rate spike, pod crash loop, deployment failure
- [ ] Incident record created with log snapshot (last 500 lines at time of incident)
- [ ] AI explanation via Anthropic API: structured JSON response (probable cause, severity, remediation steps)
- [ ] UI: incident panel on deployment detail page with AI explanation
- [ ] User-provided API key: workspace setting for Anthropic API key (AutoStack does not pay for AI calls)

### 4.2 — Resource Right-Sizer
- [ ] Analyze actual CPU/memory utilization over last 30 days per deployment
- [ ] If consistent under-utilization detected (< 20% average), generate recommendation
- [ ] Show: "This deployment uses 12% CPU on average. Reducing from 0.5 vCPU to 0.25 vCPU would save ~$18/month"
- [ ] One-click apply recommendation (triggers rollout update with new compute values)

### 4.3 — Docker Compose → AutoStack Converter
- [ ] UI: paste Docker Compose YAML → AI converts to AutoStack deployment spec
- [ ] Conversion handles: image, ports, environment, volumes, resources (with sensible defaults)
- [ ] Output shown as preview before user accepts
- [ ] Multi-service compose files generate multiple deployment specs

---

## Phase 5 — Advanced Deployment Features

**Objective**: Progressive delivery, preview environments, migration paths.

**Dependency**: Phase 2 complete (multi-provider needed for full value)

**Duration estimate**: 6-8 weeks

### 5.1 — Preview Environments (Ephemeral Deployments)
- [ ] GitHub App installation flow
- [ ] Webhook: PR opened → create ephemeral deployment on cheapest available target (Cloud Run preferred)
- [ ] Webhook: PR closed/merged → destroy ephemeral deployment
- [ ] Ephemeral deployment TTL: max 7 days regardless of PR state
- [ ] Cost limit: ephemeral deployments capped at workspace ephemeral budget setting
- [ ] UI: deployment badge in GitHub PR with live URL

### 5.2 — Traffic Splitting (Cloud Run First)
- [ ] UI: traffic split slider (10% new / 90% old)
- [ ] API: `POST /api/v1/rollouts/:id/traffic-split` with percentages
- [ ] Cloud Run: native traffic splitting implementation
- [ ] AWS ECS: requires two task definitions + weighted routing (more complex, Phase 5.2b)
- [ ] Auto-rollback if error rate on new revision exceeds threshold during split

### 5.3 — Migration Path Tool
- [ ] UI: "Migrate to Cloud" for existing Kubernetes deployments
- [ ] Flow: parallel deploy (same app on K8s + Cloud Run simultaneously) → DNS cutover → optional K8s cleanup
- [ ] Migration status view: shows both targets running, DNS propagation status

### 5.4 — Azure ACA Provider
- [ ] Implement `AzureACAProvider`
- [ ] All Provider interface methods
- [ ] Azure-specific: resource groups, container apps environments

---

## Phase 6 — Scale and Reliability

**Objective**: Platform handles 1,000+ concurrent users, 10,000+ deployments.

**Dependency**: Phase 3 complete

**Duration estimate**: ongoing

### 6.1 — PostgreSQL Migration
- [ ] Migrate PocketBase from SQLite to PostgreSQL mode
- [ ] Zero-downtime migration with litestream → PostgreSQL replication bridge
- [ ] Connection pooling (PgBouncer)

### 6.2 — WebSocket Horizontal Scaling
- [ ] Redis Pub/Sub for WebSocket event distribution across multiple backend instances
- [ ] Sticky sessions for WebSocket connections (or upgrade to stateless via Redis)

### 6.3 — Observability Platform Integration
- [ ] Prometheus metrics endpoint per deployment
- [ ] Grafana Cloud integration (automated dashboard provisioning)
- [ ] OpenTelemetry tracing from AutoStack backend itself

---

## Non-Roadmap Items (Explicitly Deferred or Out of Scope)

| Item | Decision |
|---|---|
| StatefulSet support in Kubernetes | Out of scope v1 — stateless workloads only |
| AWS Lambda / serverless functions | Out of scope — container-focused platform |
| Built-in CI/CD pipeline | Out of scope — integrate with GitHub Actions via webhook/API |
| Self-hosted Kubernetes management (Rancher-style) | Out of scope — AutoStack connects to existing clusters |
| Multi-region load balancing | Deferred to Phase 5+ |
| FedRAMP compliance | Not in scope for current planning horizon |
| Windows containers | Not in scope |
| ARM architecture optimization | Accepted limitation for now |

---

## Dependency Map

```
Phase 0 (Foundation)
    └── Phase 1 (Cloud Run)
            └── Phase 2 (ECS Fargate)
            └── Phase 3 (Org/RBAC) [parallel-friendly after Phase 1]
            └── Phase 4 (AI) [requires real incident data from Phase 1+]
            └── Phase 5 (Advanced Deployments) [requires Phase 2]
                    └── Phase 6 (Scale) [ongoing]
```

---

## Technical Debt Acknowledgment

| Debt Item | When to Address | Risk |
|---|---|---|
| cluster-admin RBAC on operator | Phase 0.1 | High — blocks enterprise sales |
| Deployment secrets in rollout spec JSON | Phase 0.4 | High — compliance risk |
| Single PocketBase instance (no HA) | Phase 6.1 | Medium — acceptable at current scale |
| Metrics not stored long-term | Phase 6.3 | Low — users can use Prometheus |
| Blueprint versioning not implemented | Phase 3 | Medium — silent breakage for blueprint users |
| Preview environment cleanup if webhook missed | Phase 5.1 | Medium — TTL mitigates |
