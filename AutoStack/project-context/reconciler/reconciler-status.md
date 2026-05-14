# Cloud Reconciler - Status

## Last Updated
2026-05-14 (Phase 1.9 principal review)

## Implementation Status
**IMPLEMENTED — status-poller only.** Code written, compiles. **No code
path calls `Provider.Deploy`**; the reconciler observes state, it does
not converge state. Acts as the readout half of a control loop whose
write half is missing. See [[lifecycle-assumptions]] and
[[reconciliation-guarantees]] for the operational semantics, and
[[deferred-operational-hardening]] Tier 1 for the dispatch gap.

## Architecture

### Location
`/pocketbase/pkg/reconciler/cloud.go`

### Config
```go
type Config struct {
    Interval       time.Duration // Default 30 seconds
    MaxConcurrency int           // Max concurrent reconciliations per provider
}

func DefaultConfig() Config {
    return Config{
        Interval:       30 * time.Second,
        MaxConcurrency: 10,
    }
}
```

## How It Works

1. **Startup**: Registers Cloud Run provider via `providers.RegisterProvider()`
2. **Polling Loop**: Runs every 30 seconds (configurable)
3. **Query**: Finds all `deployment_targets` with non-kubernetes `target_type`
4. **Reconciliation**: For each target, calls provider's `GetStatus()`
5. **Update**: Writes status back to PocketBase

## Query Logic

```sql
SELECT deployment_targets.*, rollouts.target_type, rollouts.target_config, 
       rollouts.id as rollout_id, cloud_accounts.credentials_encrypted, 
       cloud_accounts.provider, cloud_accounts.region
FROM deployment_targets
JOIN rollouts ON rollouts.id = deployment_targets.rollout
JOIN cloud_accounts ON cloud_accounts.id = deployment_targets.cloud_account
WHERE rollouts.target_type != 'kubernetes'
```

## Safety Features

### Nil-Safe Type Assertions
```go
targetID, ok := row["id"].(string)
if !ok || targetID == "" {
    log.Printf("Skipping target: missing or null id")
    return
}
```
- Prevents panics on null database values
- Logs skipped targets instead of crashing

### Error Sanitization
```go
func sanitizeError(errMsg string) string {
    sensitivePatterns := []string{
        "credential", "secret", "token", "key", "password",
        "private_key", "service_account", "access_key", "api_key",
    }
    // ... redaction logic
}
```
- Redacts sensitive data before logging
- Limits error message length to prevent log injection

### Graceful Shutdown
```go
type Reconciler struct {
    app    *pb.PocketBase
    config Config
    stopCh chan struct{}
}

func (r *Reconciler) Stop() {
    log.Println("Stopping cloud reconciler")
    close(r.stopCh)
}
```

## Provider Mapping

```go
func providerToProviderName(provider string) string {
    switch provider {
    case "gcp":   return providers.ProviderGCPCloudRun
    case "aws":   return providers.ProviderAWSECS
    case "azure": return providers.ProviderAzureACA
    default:      return ""
    }
}
```

## Update Logic

1. `updateTargetStatus()` - Updates `deployment_targets.status`, `last_synced`, `drift_summary`
2. `updateRolloutStatus()` - Updates `rollouts.status`, `last_deployed` (only if changed)

## What's Missing / Incomplete (as of Phase 1.9)

### 1. Cloud Deploy dispatch (Tier-1 gap)
**No code path calls `Provider.Deploy`.** `HandleRolloutCreate` invokes
`k8s.CreateOrUpdateRollout` unconditionally. The reconciler is a status
poller, not a converger. See [[deferred-operational-hardening]].

### 2. Distributed Lock / Leader Election
Multiple backend instances would race. SQLite WAL mode serializes
writes today; a Postgres path would not. See [[restart-behavior]].

### 3. Operation persistence
`Operation` exists in code; no PocketBase collection persists it. Crash
mid-Deploy (once Deploy dispatch lands) would be unrecoverable.

### 4. `deployment_history` writer
Migration creates the collection with immutability rules; nothing
inserts rows.

### 5. Stuck-state detection
state-model.md describes thresholds; no `last_state_change_at` column,
no detector.

## Resolved in Phase 1.9
- Circuit breaker per-target — present.
- Exponential cycle-level backoff — present and now correctly triggered
  by per-target failures (was unreachable previously).
- Failure-map data race — fixed.
- Panic recovery — scoped to single target; whole-map reset removed.
- Provider singleton mutable state — removed.
- Misleading `MaxConcurrency` config — removed.
- Placeholder `targets_queued=...` log — replaced.

## Testing Status

- [x] Code compiles (pending Cloud Run provider fix)
- [x] Import path corrected (`github.com/pocketbase/dbx`)
- [ ] Unit tests not written
- [ ] Integration tests not written
- [ ] Cannot test runtime behavior due to build failure

## Scalability Notes

Current implementation is single-threaded. For large deployments:
- Worker pool needed
- Per-provider concurrency limits needed
- Batch queries for efficiency

## Relationship to Kubernetes

**ISOLATED** - Reconciler only processes `target_type != 'kubernetes'`
- Kubernetes deployments handled by K8s watcher pool
- No overlap between reconciler and k8s package
- Safe to run alongside Kubernetes operations

## Next Steps

1. Fix Go module resolution
2. Add circuit breaker tracking
3. Implement worker pool for concurrency
4. Add distributed lock for HA
5. Write tests