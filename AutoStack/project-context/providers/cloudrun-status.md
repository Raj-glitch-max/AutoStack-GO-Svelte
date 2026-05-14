# Cloud Run Provider - Status

## Last Updated
2026-05-14

## Implementation Status
**FUNCTIONAL** — code compiles, two critical bugs fixed in Phase 2.2

## Provider Details

### Location
`pkg/providers/cloudrun/provider.go`

### Implemented Methods (Phase 2.1)
- [x] ValidateCredentials — Tests GCP credentials via ListServices
- [x] Deploy — Creates/updates Cloud Run service with min_instances scaling
- [x] GetStatus — Maps Ready/Conditions to deployment_targets status
- [x] GetMetrics — Returns ErrNotImplemented (honest refusal)
- [x] StreamLogs — Returns log streaming not implemented
- [x] EstimateCost — Static placeholder per ADR-010; documented as such
- [x] GetActualCost — Returns not implemented error
- [x] Destroy — Deletes Cloud Run service; idempotent (NOT_FOUND → nil)
- [x] ListRegions — Full GCP region list
- [x] CheckQuotas — Returns ErrNotImplemented (honest refusal)
- [x] Rollback — Returns ErrNotImplemented (previous impl was unsafe)

## Bugs Fixed in Phase 2.2

### 1. CreateServiceRequest ServiceId Conflict (FIXED)
GCP API internal validation rejects CreateServiceRequest when both
`Service.Name` (fully-qualified path) and `ServiceId` (short name) are set.
Fix: drop ServiceId; GCP derives it from the trailing segment of Service.Name.

### 2. target_config Not Threaded to Deploy (FIXED)
PocketBase JSON columns return `map[string]interface{}`, not `string`.
Original type-assertion to string always failed. Fix: type switch handling
map/string/[]byte, plus thread targetConfig through to DeploySpec.TargetConfig.

## SDK Dependencies
- `cloud.google.com/go/run/apiv2` — Cloud Run API v2 client
- `google.golang.org/api` — Google API core

## Known Provider Limitations

| Limitation | Severity | Workaround |
|---|---|---|
| No GCP Secret Manager integration | MEDIUM | Secrets passed as plaintext env vars |
| No pre-destroy confirm poll | LOW | Provider.Destroy returns on API 200; NOT_FOUND check deferred |
| No Rollback support | MEDIUM | Rollback not yet operational |
| No Log streaming | LOW | Log streaming returns not-implemented |
| No live cost API | LOW | Static estimates only; documented |
| No quota check | LOW | CheckQuotas returns ErrNotImplemented |

## Credential Structure

```go
type CloudRunCredentials struct {
    ServiceAccountJSON string `json:"service_account_json"` // GCP service account JSON
    ProjectID          string `json:"project_id"`          // GCP project ID
}
```

Expects JSON blob in `credentials_encrypted` field with both fields.

## Security Notes

- Credentials decrypted in-memory per-request; never logged
- `CloudAccount.String()` / `GoString()` redact CredentialsEncrypted
- Error messages sanitized before logging — no credential exposure
- Service account JSON stored AES-256-GCM encrypted at rest

## Assumptions Verified

1. **Cloud Run API v2** is the current GCP API — verified used
2. **Service account JSON format** — requires valid OAuth2 service account
3. **Region list** — current as of 2024; reasonably complete
4. **Cost estimates** — documented as hardcoded estimates; not promises

## Next Steps

1. [MEDIUM] Implement GCP Secret Manager integration for secrets
2. [LOW] Add pre-destroy NOT_FOUND confirmation polling
3. [LOW] Implement Rollback via TrafficTargets
4. [LOW] Upgrade EstimateCost to call GCP Cloud Billing API
5. [LOW] Add runtime sweep goroutine for operation expiry