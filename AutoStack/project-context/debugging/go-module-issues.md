# Go Module Resolution Issues

## Last Updated
2025-05-13 (Updated: RESOLVED)

## Issue Summary

**Status**: RESOLVED

The `cloud.google.com/go/run` package import issue has been resolved.

## Resolution

The issue was caused by using the wrong import path. The Cloud Run SDK v1.21.0 has a different structure:

1. **Root package**: `cloud.google.com/go/run` does not export types directly
2. **Generated client**: `cloud.google.com/go/run/apiv2` contains the generated code
3. **Proto types**: `cloud.google.com/go/run/apiv2/runpb` contains protobuf types

### Solution

Changed import from:
```go
import (
    "cloud.google.com/go/logging"
    "cloud.google.com/go/run"
    runpb "cloud.google.com/go/run/apiv2"
)
```

To:
```go
import (
    "cloud.google.com/go/run/apiv2"
    runpb "cloud.google.com/go/run/apiv2/runpb"
)
```

### API Changes Required

The Cloud Run v2 API has different struct names and field types:

1. **EnvVar**: Changed from `Value` string to `Values` oneof with `EnvVar_Value` wrapper
2. **Service status**: Changed from `service.Status.Conditions` to `service.Conditions`
3. **Service URL**: Changed from `service.Status.Url` to `service.Uri`
4. **Condition states**: Changed from `ConditionState_TRUE/FALSE` to `CONDITION_SUCCEEDED/FAILED`
5. **Template**: Changed from `ReviewTemplate` to `RevisionTemplate`
6. **Scaling**: `MinInstanceCount` is `int32`, not `int64`

## Environment Details

- Go Version: 1.25.0
- Module Name: `github.com/janlauber/one-click`
- SDK Version: `cloud.google.com/go/run v1.21.0`

## Additional Fixes Applied

1. **dbx API correction**: `Join()` requires 3 arguments (type, table, expression)
2. **dbx.Select()**: Added proper parameter passing to `All()`
3. **PocketBase hooks**: Simplified `StartReconcilerOnBoot()` to direct call after bootstrap
4. **Collection lookup**: Changed `FindCollectionByName` to `FindCollectionByNameOrId`
5. **Removed unused imports**: Cleaned up `encoding/json`, `fmt`, `apis`, `core` from cloudAccounts.go

## Verification

```bash
cd pocketbase
go build ./...
# Build successful with no errors
```

## Related Files Changed

- `pkg/providers/cloudrun/provider.go` - Fixed import paths and API usage
- `pkg/reconciler/cloud.go` - Fixed dbx query syntax
- `pkg/controller/cloudAccounts.go` - Removed unused imports, fixed collection lookup

## Lessons Learned

1. Google Cloud SDK packages restructure frequently - always check the actual module version
2. Generated API clients have different structure than manual SDK packages
3. Proto-generated code uses oneof patterns for optional fields
4. Condition states vary by API version (v1 vs v2)