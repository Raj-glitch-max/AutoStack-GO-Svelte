# Current Architecture - AutoStack

## Last Updated
2025-05-13

## High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                          USER BROWSER                                 │
│                       SvelteKit Frontend                              │
└─────────────────────────────┬────────────────────────────────────────┘
                              │ HTTP REST + WebSocket
┌─────────────────────────────▼────────────────────────────────────────┐
│                        GO BACKEND (single process)                    │
│  ┌───────────────┐  ┌──────────────┐  ┌──────────────────────────┐   │
│  │  REST API     │  │  WebSocket   │  │  Background Services      │   │
│  │  /api/v1/...  │  │  Hub         │  │  ┌────────────────────┐   │   │
│  └───────┬───────┘  └──────┬───────┘  │  │ Auto-Update Sched  │   │   │
│          │                 │          │  └────────────────────┘   │   │
│  ┌────────┴────────────────┴────────┐ │  ┌────────────────────┐   │   │
│  │        PocketBase                │ │  │ Cloud Reconciler   │   │   │
│  │   (Auth + Data + File Storage)   │ │  └────────────────────┘   │   │
│  └────────────────┬─────────────────┘ └──────────────────────────┘   │
│                   │                                                 │
│  ┌────────────────▼─────────────────┐                               │
│  │        Provider Layer            │                               │
│  │  ┌──────┐ ┌──────┐ ┌──────────┐  │                               │
│  │  │  K8s │ │  ECS │ │CloudRun  │  │ ← NEW (in progress)           │
│  │  └──────┘ └──────┘ └──────────┘  │                               │
│  └──────────────────────────────────┘                               │
└──────────────────────────────────────────────────────────────────────┘
         │              │               │
         ▼              ▼               ▼
   Kubernetes      AWS ECS         Google Cloud
   Cluster(s)      Fargate          Run
```

## Components

### Existing (Working)
- **SvelteKit Frontend**: `/frontend/` - User interface
- **Go Backend**: `/pocketbase/` - Single process, multiple goroutines
- **PocketBase**: Data + Auth layer with SQLite
- **Kubernetes Watcher Pool**: `/pkg/watcher/` - Real-time K8s events
- **Rollout Controllers**: `/pkg/controller/rollouts.go` - K8s deployment management
- **K8s Operations**: `/pkg/k8s/` - Kubernetes API interactions

### New (Implemented but Untested)
- **Provider Interface**: `/pkg/providers/provider.go`
- **Cloud Run Provider**: `/pkg/providers/cloudrun/provider.go`
- **Cloud Reconciler**: `/pkg/reconciler/cloud.go`
- **Cloud Account Controllers**: `/pkg/controller/cloudAccounts.go`

### New (In Design)
- **ECS Provider**: Not yet implemented
- **Azure ACA Provider**: Not yet implemented

## Data Flows

### Kubernetes Deployment (UNCHANGED)
```
User → Frontend → POST /api/v1/rollouts → PocketBase
                                        ↓
                               HandleRolloutCreate (controller)
                                        ↓
                               k8s.CreateOrUpdateRollout
                                        ↓
                               Kubernetes CRD
                                        ↓
                               Kubernetes Operator
                                        ↓
                               k8s watcher pool → WebSocket → Frontend
```

### Cloud Deployment (NEW - NOT TESTED)
```
User → Frontend → POST /api/v1/rollouts → PocketBase
                                        ↓
                               HandleRolloutCreate (controller)
                                        ↓
                               Writes deployment_targets record
                                        ↓
                               Cloud Reconciler (30s polling)
                                        ↓
                               Provider.Deploy()
                                        ↓
                               Cloud API
                                        ↓
                               Status updates → PocketBase → Frontend
```

## Provider Interface Contract

```go
type Provider interface {
    ValidateCredentials(ctx context.Context, account *CloudAccount) error
    Deploy(ctx context.Context, account *CloudAccount, spec *DeploySpec) (*DeployResult, error)
    GetStatus(ctx context.Context, account *CloudAccount, target *DeploymentTarget) (*TargetStatus, error)
    GetMetrics(ctx context.Context, account *CloudAccount, target *DeploymentTarget) (*TargetMetrics, error)
    StreamLogs(ctx context.Context, account *CloudAccount, target *DeploymentTarget, writer io.Writer) error
    EstimateCost(ctx context.Context, account *CloudAccount, spec *DeploySpec) (*CostEstimate, error)
    GetActualCost(ctx context.Context, account *CloudAccount, target *DeploymentTarget, startDate, endDate time.Time) (*ActualCost, error)
    Destroy(ctx context.Context, account *CloudAccount, target *DeploymentTarget) error
    ListRegions(ctx context.Context, account *CloudAccount) ([]string, error)
    CheckQuotas(ctx context.Context, account *CloudAccount, spec *DeploySpec) (*QuotaCheck, error)
}
```

## Package Boundaries

- `pkg/providers/` - Provider implementations ONLY
- `pkg/reconciler/` - Reconciliation logic ONLY
- `pkg/controller/` - HTTP handlers + PocketBase hooks
- `pkg/k8s/` - Kubernetes operations ONLY (UNCHANGED)
- `pkg/watcher/` - WebSocket event streaming (UNCHANGED)

## Critical Invariants

1. Kubernetes path NEVER touches cloud code
2. Cloud code NEVER modifies Kubernetes resources
3. PocketBase is single source of truth for both
4. All cloud SDK calls isolated to provider packages
5. Credentials never logged (enforced via error sanitization)

## API Routes

### Existing (Kubernetes)
- `GET /pb/:projectId/:deploymentId/status`
- `GET /pb/:projectId/:deploymentId/metrics`
- `GET /pb/:projectId/:deploymentId/events`
- `GET /ws/k8s/deployments`
- `GET /ws/k8s/logs`

### New (Cloud) - Routes Added
- `POST /api/v1/cloud-accounts` - Create cloud account
- `GET /api/v1/cloud-accounts` - List accounts
- `POST /api/v1/cloud-accounts/:id/validate` - Validate credentials
- `GET /api/v1/cloud-accounts/:id/regions` - List regions
- `POST /api/v1/cost/estimate` - Estimate deployment cost

## Known Architecture Risks

1. **Go Module Resolution** - Cloud Run SDK has import issues
2. **Reconciler Scaling** - Single-threaded polling may need worker pool for large deployments
3. **No Circuit Breaker** - Failed API calls will retry indefinitely
4. **No Distributed Lock** - Multiple instances would race

## Scaling Path (Documented for Future)

1. Backend: horizontal scaling behind load balancer + Redis for WebSocket affinity
2. PocketBase: migrate to PostgreSQL for concurrent writes
3. Cloud Reconciler: distributed lock via PocketBase-based locking
4. WebSocket: Redis Pub/Sub for cross-instance events