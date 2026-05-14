# ARCHITECTURE.md — AutoStack Technical Architecture

---

## Architecture Overview

AutoStack is a **modular monolith** with a clear internal service boundary between components. It is not microservices. All Go code runs in a single process with multiple goroutines. The frontend is a separate SvelteKit process.

```
┌──────────────────────────────────────────────────────────────────────┐
│                          USER BROWSER                                 │
│                       SvelteKit Frontend                              │
│          (WebSocket client + REST client + real-time UI)              │
└─────────────────────────────┬────────────────────────────────────────┘
                              │ HTTP REST + WebSocket
┌─────────────────────────────▼────────────────────────────────────────┐
│                        GO BACKEND (single process)                    │
│  ┌───────────────┐  ┌──────────────┐  ┌──────────────────────────┐   │
│  │  REST API     │  │  WebSocket   │  │  Background Services      │   │
│  │  /api/v1/...  │  │  Hub         │  │  ┌────────────────────┐   │   │
│  │               │  │              │  │  │ K8s Watcher Pool   │   │   │
│  └───────┬───────┘  └──────┬───────┘  │  └────────────────────┘   │   │
│          │                 │          │  ┌────────────────────┐   │   │
│          └────────┬────────┘          │  │ Cloud Reconciler   │   │   │
│                   │                   │  └────────────────────┘   │   │
│  ┌────────────────▼─────────────────┐ │  ┌────────────────────┐   │   │
│  │        PocketBase                │ │  │ Auto-Update Sched  │   │   │
│  │   (Auth + Data + File Storage)   │ │  └────────────────────┘   │   │
│  └────────────────┬─────────────────┘ │  ┌────────────────────┐   │   │
│                   │ SQLite/PostgreSQL  │  │ Cost Sync Service  │   │   │
│                   │                   │  └────────────────────┘   │   │
│  ┌────────────────▼─────────────────┐ └──────────────────────────┘   │
│  │        Provider Layer             │                                 │
│  │  ┌──────┐ ┌──────┐ ┌──────────┐  │                                 │
│  │  │  K8s │ │  ECS │ │CloudRun  │  │                                 │
│  │  └──────┘ └──────┘ └──────────┘  │                                 │
│  └──────────────────────────────────┘                                 │
└──────────────────────────────────────────────────────────────────────┘
         │              │               │
         ▼              ▼               ▼
   Kubernetes      AWS ECS         Google Cloud
   Cluster(s)      Fargate          Cloud Run
```

---

## Components

### 1. SvelteKit Frontend

**Purpose**: The user interface. All business logic lives in the backend. The frontend is a thin client.

**Responsibilities**:
- Render deployment dashboards, forms, history views
- Maintain a WebSocket connection per active project view
- Stream log lines as they arrive from the WebSocket
- Display real-time metrics and status updates
- Make REST API calls for all data mutations and queries
- Handle auth tokens from PocketBase (JWT-based)

**Must Not**:
- Contain any business logic
- Make direct Kubernetes or cloud provider API calls
- Store credentials
- Calculate costs
- Make decisions about deployment configuration validity

**Communication**:
- REST API → Go backend `/api/v1/`
- WebSocket → Go backend `/ws/`
- Auth → PocketBase `/api/collections/users/auth-with-password` etc.

---

### 2. Go Backend (REST API Layer)

**Purpose**: The application logic layer. Handles all HTTP requests from the frontend.

**Responsibilities**:
- Authenticate and authorize every request (via PocketBase JWT validation)
- Validate all incoming request payloads
- Translate frontend actions into Provider operations or PocketBase mutations
- Serve deployment configuration, history, cost estimates
- Issue API keys for programmatic access
- Route deployment operations to the correct Provider implementation

**Must Not**:
- Make direct cloud SDK calls (delegated to Provider layer)
- Contain deployment reconciliation logic (delegated to Cloud Reconciler)
- Contain Kubernetes watcher logic (delegated to K8s Watcher Pool)
- Block on long-running operations (return job IDs and let clients poll/subscribe)

**Key API Groups**:
```
/api/v1/auth/           → API key management
/api/v1/projects/       → Project CRUD
/api/v1/rollouts/       → Deployment CRUD and operations
/api/v1/rollouts/:id/rollback     → Rollback to history version
/api/v1/rollouts/:id/logs         → Log streaming initiation
/api/v1/cloud-accounts/           → Cloud account management
/api/v1/cost/estimate             → Pre-deployment cost estimation
/api/v1/blueprints/               → Blueprint library
/api/v1/organizations/            → Org/workspace management (planned)
/api/v1/ai/explain-incident       → AI incident analysis
```

---

### 3. PocketBase (Data and Auth Layer)

**Purpose**: Single source of truth for all desired state. Auth provider.

**Responsibilities**:
- Store all deployment desired state (rollout specs for both Kubernetes and cloud)
- Store rollout history (immutable append-only history records)
- Store user accounts, sessions, OAuth2 tokens
- Store cloud account credentials (AES-256 encrypted fields)
- Store registry credentials (AES-256 encrypted fields)
- Store blueprints, auto-update policies, organization structure
- Store audit logs (append-only collection)
- Store cost records, DNS records, network configs

**Technology**: PocketBase with SQLite (current). Migration to PostgreSQL mode available when scale requires it.

**Critical Architecture Rule**: PocketBase is the source of truth for desired state. The Kubernetes CRD reflects PocketBase state, not the reverse. The cloud reconciler drives toward PocketBase state. No component writes desired state changes directly to Kubernetes or cloud providers without first writing to PocketBase.

**Access Pattern**: Backend accesses PocketBase via Go SDK (embedded PocketBase library, not separate process). Frontend accesses auth endpoints directly via PocketBase's built-in auth API.

---

### 4. Kubernetes Watcher Pool

**Purpose**: Maintain real-time awareness of Kubernetes cluster state.

**Responsibilities**:
- For each active user session watching a project, maintain an active Kubernetes watcher
- Watch Pods, Deployments, Services, Ingresses, Jobs, Events in the project's namespace
- Convert Kubernetes watch events into normalized `WatchEvent` structs
- Push events to the WebSocket Hub for delivery to the correct client connection

**Technology**: Kubernetes `watch` API via `k8s.io/client-go`. Standard `SharedInformer` pattern.

**Lifecycle**:
- Watcher starts when a user opens a project in the frontend
- Watcher stops when all users disconnect from that project's WebSocket channel
- Watchers are namespace-scoped (not cluster-scoped) for isolation

**Note**: This component is complete and production-ready. See `KUBERNETES_EXISTING_SYSTEM.md`.

---

### 5. Cloud Reconciler

**Purpose**: The cloud equivalent of the Kubernetes operator. Actively manages cloud deployments toward desired state.

**Responsibilities**:
- On a configurable interval (default: 30 seconds), poll PocketBase for all active cloud deployments
- For each active cloud deployment, call Provider.GetStatus() to get actual state
- Compare desired state (from PocketBase) with actual state (from cloud API)
- If diverged: call Provider.Deploy() or appropriate correction action
- Update PocketBase with actual state after polling
- Detect drift: flag cases where actual state diverged from desired state without AutoStack action
- Push synthesized WebSocket events for cloud deployments (so frontend has consistent experience)

**Technology**: Background goroutine with distributed lock (PocketBase-based lock to prevent multiple instances running simultaneously)

**Critical**: This is the component that makes AutoStack a platform, not just a deployment tool. Without it, cloud deployments are fire-and-forget. With it, AutoStack actively manages cloud workloads exactly as the Kubernetes operator manages pods.

**Cloud-specific timing**: Unlike Kubernetes watchers (event-driven, sub-second), cloud reconciliation is polling-based (30-second intervals). The frontend must display cloud status with appropriate staleness indicators.

---

### 6. Provider Layer

**Purpose**: Abstraction that makes all cloud targets interchangeable.

**The Provider Interface** (Go):
```go
type Provider interface {
    // Validate that credentials are correct and have sufficient permissions
    ValidateCredentials(ctx context.Context, account CloudAccount) error

    // Deploy or update an application. Idempotent.
    Deploy(ctx context.Context, spec DeploySpec) (*DeployResult, error)

    // Get current actual status from the cloud provider
    GetStatus(ctx context.Context, target DeploymentTarget) (*TargetStatus, error)

    // Get current live metrics (CPU, memory)
    GetMetrics(ctx context.Context, target DeploymentTarget) (*TargetMetrics, error)

    // Stream logs to the provided writer
    StreamLogs(ctx context.Context, target DeploymentTarget, writer io.Writer) error

    // Estimate cost BEFORE deploying, using live pricing APIs
    EstimateCost(ctx context.Context, account CloudAccount, spec DeploySpec) (*CostEstimate, error)

    // Get actual incurred cost from billing API (where available)
    GetActualCost(ctx context.Context, account CloudAccount, target DeploymentTarget) (*ActualCost, error)

    // Destroy all resources for this deployment. Idempotent.
    Destroy(ctx context.Context, target DeploymentTarget) error

    // List available regions for this provider
    ListRegions(ctx context.Context, account CloudAccount) ([]string, error)

    // Validate that the account has sufficient quotas for this DeploySpec
    CheckQuotas(ctx context.Context, account CloudAccount, spec DeploySpec) (*QuotaCheck, error)
}
```

**Implementations**:
- `KubernetesProvider` — wraps the existing operator (mostly a no-op since K8s path is separate)
- `AWSECSProvider` — AWS ECS Fargate
- `GCPCloudRunProvider` — Google Cloud Run
- `AzureACAProvider` — Azure Container Apps
- `AWSEKSProvider` — AWS EKS (planned, not in initial scope)

**Location**: `/pkg/providers/`

**Critical Rule**: All cloud SDK calls (AWS SDK, GCP SDK, Azure SDK) live ONLY inside provider implementations. No SDK calls anywhere else.

---

### 7. Auto-Update Scheduler

**Purpose**: Poll container registries for new image tags and trigger updates.

**Responsibilities**:
- Maintain a list of active auto-update policies from PocketBase
- For each policy, poll the relevant container registry API on the configured interval
- Compare available tags against the current deployed tag using the policy's matching rules
- If a qualifying new tag is found: trigger a new rollout via the Provider for the relevant target type
- Record the update in rollout history
- Respect update windows (time-based scheduling)

**Technology**: Goroutine with ticker. Uses registry-credential-aware registry clients.

**Already implemented for Kubernetes path**. Needs extension to call cloud Provider for cloud deployments.

---

### 8. Auto-Update Scheduler

### 8. Cost Sync Service

**Purpose**: Periodically fetch actual cost data from cloud billing APIs and store in PocketBase.

**Responsibilities**:
- Daily: call AWS Cost Explorer API / GCP Billing API / Azure Cost Management API per connected cloud account
- Attribute costs to specific deployments using resource tags (autostack/deployment-id)
- Store daily cost records in `cost_records` collection
- Detect cost anomalies (daily cost > 2x rolling 7-day average)
- Trigger anomaly notifications via notification system

**Technology**: Background goroutine with daily ticker.

---

### 9. WebSocket Hub

**Purpose**: Route real-time event messages to the correct client connections.

**Responsibilities**:
- Maintain a registry of active WebSocket connections keyed by user session and project ID
- Receive events from: Kubernetes Watcher Pool (real-time push) and Cloud Reconciler (synthetic polling events)
- Route events to the correct connected clients based on project membership
- Handle client disconnection cleanup

**Message Format** (consistent across all target types):
```json
{
  "type": "pod_event | deployment_event | log_line | metrics_update | cost_update | drift_detected",
  "project_id": "...",
  "rollout_id": "...",
  "target_type": "kubernetes | ecs | cloudrun | aca",
  "payload": { ... }
}
```

---

### 10. Notification Dispatcher

**Purpose**: Send notifications across all configured channels.

**Technology**: Novu API (managed notification infrastructure) or self-hosted.

**Channels**: Email, Slack, Teams, webhook (generic), in-app notification.

**Trigger events**:
- Deployment completed (success or failure)
- Rollback triggered (automatic or manual)
- Auto-update applied
- Cost anomaly detected
- Drift detected
- Quota approaching limit
- Certificate expiring within 30 days

---

## Data Flow: Kubernetes Deployment (Existing, Working)

```
User clicks Deploy in UI
       ↓
Frontend calls POST /api/v1/rollouts
       ↓
Backend validates request, writes new RolloutSpec to PocketBase (rollouts collection)
       ↓
Backend writes (or updates) Rollout CRD in the target Kubernetes namespace
       ↓
Kubernetes Operator detects CRD change via watch
       ↓
Operator reconciles: creates/updates Deployment, Service, Ingress, HPA, PVC
       ↓
Kubernetes events flow into Watcher Pool goroutines
       ↓
Watcher Pool converts events to WatchEvent structs
       ↓
WebSocket Hub delivers events to connected frontend client
       ↓
Frontend updates UI in real time
       ↓
Operator updates Rollout status subresource when deployment completes
       ↓
Backend watches status updates, syncs PocketBase rollout status
```

---

## Data Flow: Cloud Deployment (New)

```
User clicks Deploy in UI (with cloud account selected as target)
       ↓
Frontend calls POST /api/v1/rollouts
       ↓
Backend validates request including cloud account permissions
       ↓
Backend calls Provider.CheckQuotas() — abort if quota insufficient
       ↓
Backend writes RolloutSpec + target_type + cloud_account_id to PocketBase
       ↓
Backend calls Provider.Deploy() — creates cloud resources
       ↓
Backend writes DeploymentTarget record to PocketBase with external_id
       ↓
Cloud Reconciler picks up new deployment on next polling cycle (max 30s delay)
       ↓
Reconciler calls Provider.GetStatus() every 30 seconds
       ↓
Reconciler synthesizes WatchEvent messages from polling results
       ↓
WebSocket Hub delivers synthesized events to connected frontend
       ↓
Frontend updates UI (with cloud provider indicator)
       ↓
When deployment completes: Reconciler updates PocketBase status to "running"
       ↓
Notification sent to user via configured notification channel
```

---

## Data Flow: Rollback (Both Targets)

```
User selects a history version and clicks Rollback
       ↓
Frontend calls POST /api/v1/rollouts/:id/rollback with history_id
       ↓
Backend fetches rollout_history record (the snapshot to roll back to)
       ↓
Backend writes new rollout spec to PocketBase (overwriting current spec)
       ↓
Backend records a new rollout_history entry with change_type: rollback
       ↓
For Kubernetes: Backend updates CRD → Operator reconciles → K8s rolls back
For Cloud: Cloud Reconciler detects spec change → Provider.Deploy() with old spec
       ↓
Rollback progress tracked via same real-time events as a normal deployment
```

---

## Architectural Constraints

### Constraint 1: No `if provider == "aws"` in Core Paths
All provider-specific logic lives in provider implementations. The API layer, reconciler, and scheduler work with the Provider interface only. Violations of this rule cause fragmentation that is impossible to maintain.

### Constraint 2: PocketBase Is Always Written Before Cloud API Is Called
Every deployment action writes to PocketBase first, then calls the cloud API. This means if the cloud API call fails, PocketBase has the desired state and the reconciler will retry. If PocketBase write fails, no cloud resources are created. Desired state is never only in cloud.

### Constraint 3: Long-Running Operations Return Immediately
Deploy, Destroy, and other cloud operations that take minutes do not block the HTTP response. The API responds with a job or operation ID. The client subscribes to WebSocket events for progress. This prevents HTTP timeouts on slow cloud APIs.

### Constraint 4: Credentials Are Never Logged
All logging middleware must filter fields named: password, secret, token, key, credential, api_key, access_key, private_key. This filtering is applied at the structured logger level, not per-callsite.

### Constraint 5: The Kubernetes Operator Has No Knowledge of Cloud
The Kubernetes operator watches Kubernetes CRDs. It does not know about PocketBase cloud accounts, cloud deployment targets, or the Provider interface. It is a pure Kubernetes controller. Cloud is entirely separate.

### Constraint 6: Single Source of Truth
Desired state lives in PocketBase. Actual state is observed from the cluster or cloud provider. The reconciler's job is to close the gap. No component assumes desired state can be read from the cluster or cloud provider.

---

## Scalability Architecture Notes

### Current Scale (Single Instance)
- One Go backend process
- One PocketBase instance (SQLite)
- One Kubernetes operator per cluster
- One cloud reconciler goroutine

### Future Scale Path
- Backend: horizontal scaling behind a load balancer (stateless HTTP + Redis for WebSocket session affinity)
- PocketBase: migrate to PostgreSQL mode for concurrent write performance
- Kubernetes Operator: remains single-instance per cluster (Kubernetes controller pattern)
- Cloud Reconciler: distributed lock pattern ensures only one instance reconciles at a time; safe to run multiple instances
- WebSocket: Redis Pub/Sub for event distribution across multiple backend instances

### Time-Series Metrics
PocketBase SQLite is not suitable for high-frequency metrics storage at scale. Architecture decision: do not store raw metrics in PocketBase. Push metrics to Prometheus-compatible endpoint. Users configure their own metrics backend (Grafana Cloud, Datadog, self-hosted Prometheus). AutoStack provides metrics in Prometheus format via `/metrics/` endpoint per deployment.

---

## Technology Stack Summary

| Component | Technology | Rationale |
|---|---|---|
| Backend | Go | Performance, operator ecosystem, strong concurrency |
| Frontend | SvelteKit | Reactivity, performance, existing codebase |
| Database | PocketBase / SQLite → PostgreSQL | Rapid development, embedded, migration path exists |
| K8s Client | k8s.io/client-go | Standard, well-maintained |
| Cloud: AWS | aws-sdk-go-v2 | Official AWS SDK |
| Cloud: GCP | google.golang.org/api | Official GCP SDK |
| Cloud: Azure | github.com/Azure/azure-sdk-for-go | Official Azure SDK |
| Cost Estimation | AWS Pricing API + Infracost API | Real pricing data, no hardcoding |
| Notifications | Novu (self-hosted or cloud) | Multi-channel, open source |
| Secrets (platform) | Infisical or self-managed AES-256 | No external secret dependency at launch |
| Monitoring Output | Prometheus Remote Write | Universal compatibility |
| Helm | helm.sh/helm/v3 | Standard Kubernetes package management |
| AI Features | Anthropic API (user-provided key) | Claude for technical content quality |
