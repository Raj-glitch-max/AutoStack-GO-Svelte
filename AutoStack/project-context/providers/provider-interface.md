# Provider Interface - Documentation

## Location
`/pocketbase/pkg/providers/provider.go`

## Purpose

Defines the contract that all cloud provider implementations must satisfy. Enables provider-agnostic deployment management.

## Interface Definition

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

## Provider Registry

Providers register themselves at startup:
```go
func RegisterProvider(name string, p Provider)
func GetProvider(name string) (Provider, error)
```

Current providers:
- `providers.ProviderGCPCloudRun = "gcp-cloudrun"` - Implemented
- `providers.ProviderAWSECS = "aws-ecs"` - Not implemented
- `providers.ProviderAzureACA = "azure-aca"` - Not implemented

## Data Types

### CloudAccount
```go
type CloudAccount struct {
    ID                   string
    Name                 string
    User                 *pb_models.Record
    Provider             string // aws, gcp, azure
    Region               string
    CredentialsEncrypted string // JSON blob, decrypted at runtime
    Status               string // active, error, validating, revoked
    ValidatedAt          time.Time
    LastValidated        time.Time
    ValidationError      string
}
```

### DeploySpec
```go
type DeploySpec struct {
    RolloutID       string
    Image           ImageSpec
    Compute         ComputeSpec
    Scale           ScaleSpec
    Network         NetworkSpec
    Env             []EnvVar
    Secrets         []SecretRef
    Health          *HealthSpec
    TargetConfig    map[string]interface{} // Provider-specific overrides
}
```

### DeployResult
```go
type DeployResult struct {
    ExternalID    string
    EndpointURL  string
    Status       string
    Message      string
}
```

### TargetStatus
```go
type TargetStatus struct {
    Status            string
    Replicas          int
    AvailableReplicas int
    ReadyReplicas     int
    Message           string
    LastUpdated       time.Time
}
```

### CostEstimate
```go
type CostEstimate struct {
    ComputeMonthlyLow  float64
    ComputeMonthlyHigh float64
    InfrastructureMonthlyLow float64
    InfrastructureMonthlyHigh float64
    TotalMonthlyLow   float64
    TotalMonthlyHigh  float64
    UncertaintyNote   string
    PricingSource     string
    CalculatedAt      time.Time
}
```

## Error Handling

```go
var ErrProviderNotFound = &ProviderError{Code: "PROVIDER_NOT_FOUND", Message: "Provider not found"}

type ProviderError struct {
    Code    string
    Message string
}
```

## Requirements for New Providers

1. Implement all 10 methods
2. Register in reconciler startup
3. Handle credentials securely (no logging)
4. Return sanitized error messages
5. Use provider-specific SDK in isolated package

## Provider-to-PocketBase Mapping

| PocketBase provider | Provider Name |
|---------------------|--------------|
| gcp | providers.ProviderGCPCloudRun |
| aws | providers.ProviderAWSECS |
| azure | providers.ProviderAzureACA |

## Security Requirements

1. Credentials must be decrypted only in-memory
2. Credentials must never appear in logs
3. Error messages must be sanitized
4. API calls must use encrypted transport (HTTPS)