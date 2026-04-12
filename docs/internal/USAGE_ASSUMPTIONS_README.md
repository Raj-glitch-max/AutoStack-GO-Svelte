# Usage Assumptions System

## Overview

The Usage Assumptions system provides a centralized, configurable way to manage and display cost estimation assumptions for each blueprint type. This ensures transparency and allows users to understand what assumptions are being made when calculating AWS costs.

## Features

- **Centralized Management**: All usage assumptions are managed in one place
- **Per-Blueprint Configuration**: Each blueprint type has its own set of assumptions
- **Detailed Documentation**: Each assumption includes a description and unit
- **Configurable Values**: Assumptions can be updated programmatically
- **User-Friendly Display**: Formatted output for displaying assumptions to users
- **Validation**: Built-in validation for assumption values

## Architecture

### Core Components

1. **UsageAssumption**: Represents a single assumption with key, value, description, and unit
2. **UsageAssumptions**: Collection of assumptions for a blueprint type
3. **UsageAssumptionsManager**: Manages all assumptions across blueprint types

### Integration

The Usage Assumptions system integrates with:
- **BlueprintMapper**: Provides assumptions for cost estimation
- **Blueprint Calculators**: Use assumptions in cost calculations
- **API Endpoints**: Expose assumptions to frontend

## Usage Examples

### Basic Usage

```go
// Create a new usage assumptions manager
manager := NewUsageAssumptionsManager()

// Get assumptions for a blueprint type
assumptions, err := manager.GetAssumptions("static-website")
if err != nil {
    log.Fatal(err)
}

// Access assumptions as a map
assumptionsMap := assumptions.AsMap
fmt.Println("Storage:", assumptionsMap["storage"])

// Access assumptions as a list with descriptions
for _, assumption := range assumptions.Assumptions {
    fmt.Printf("%s: %s (%s)\n", 
        assumption.Description, 
        assumption.Value, 
        assumption.Unit)
}
```

### Updating Assumptions

```go
manager := NewUsageAssumptionsManager()

// Update a single assumption
err := manager.UpdateAssumption("static-website", "storage", "50 GB")
if err != nil {
    log.Fatal(err)
}

// Set multiple custom assumptions
customAssumptions := map[string]string{
    "storage":      "100 GB",
    "requests":     "100,000 per month",
    "dataTransfer": "500 GB per month",
}
err = manager.SetCustomAssumptions("static-website", customAssumptions)
if err != nil {
    log.Fatal(err)
}
```

### Displaying Assumptions

```go
manager := NewUsageAssumptionsManager()

// Get formatted display string
display, err := manager.FormatAssumptionsForDisplay("static-website")
if err != nil {
    log.Fatal(err)
}
fmt.Println(display)

// Output:
// Usage Assumptions for static-website:
//   • S3 storage for static assets: 10 GB (GB)
//   • HTTP/HTTPS requests to website: 10,000 per month (requests/month)
//   • Data transfer out via CloudFront: 100 GB per month (GB/month)
//   ...
```

### Validation

```go
manager := NewUsageAssumptionsManager()

// Validate an assumption value
err := manager.ValidateAssumptionValue("static-website", "storage", "20 GB")
if err != nil {
    fmt.Println("Invalid assumption:", err)
}

// Empty values are rejected
err = manager.ValidateAssumptionValue("static-website", "storage", "")
// Returns error: "assumption value cannot be empty"

// Invalid keys are rejected
err = manager.ValidateAssumptionValue("static-website", "invalid-key", "value")
// Returns error: "assumption key 'invalid-key' not found"
```

### Integration with Blueprint Mapper

```go
app := /* your PocketBase app */
mapper := NewBlueprintMapper(app)
manager := NewUsageAssumptionsManager()

// Get assumptions from both systems
mapperAssumptions, _ := mapper.GetAssumptions("static-website")
managerAssumptions, _ := manager.GetAssumptionsAsMap("static-website")

// Both provide assumptions for the same blueprint
fmt.Println("Mapper assumptions:", mapperAssumptions)
fmt.Println("Manager assumptions:", managerAssumptions)

// Use in cost estimation
config := map[string]interface{}{}
estimate, err := mapper.EstimateCost("static-website", config, "us-east-1")
if err != nil {
    log.Fatal(err)
}

// The estimate includes assumptions in the breakdown
breakdown := estimate.Breakdown.(*StaticWebsiteCostBreakdown)
fmt.Println("Cost assumptions:", breakdown.Assumptions)
```

## Blueprint-Specific Assumptions

### Static Website

- **storage**: S3 storage for static assets (default: 10 GB)
- **requests**: HTTP/HTTPS requests to website (default: 10,000 per month)
- **dataTransfer**: Data transfer out via CloudFront (default: 100 GB per month)
- **customDomain**: Route53 hosted zone for custom domain (default: false)
- **cacheHitRatio**: CloudFront cache hit ratio (default: 80%)
- **compressionRate**: Content compression rate (default: 60%)

### Web Application

- **compute**: Fargate task compute resources (default: 0.25 vCPU, 0.5 GB memory)
- **taskCount**: Number of Fargate tasks running (default: 1 task)
- **database**: RDS instance type and storage (default: db.t3.micro, 20 GB storage)
- **loadBalancer**: Application Load Balancer capacity units (default: 730 LCU-hours)
- **natGateway**: NAT Gateway data processing (default: 50 GB data processed)
- **cloudwatchLogs**: CloudWatch Logs ingestion and storage (default: 5 GB logs)
- **availability**: Availability zone configuration (default: Single AZ)
- **uptime**: Service uptime assumption (default: 24/7)

### Full-Stack Application

- **frontend**: Frontend Fargate service resources (default: 0.25 vCPU, 0.5 GB memory, 1 task)
- **backend**: Backend Fargate service resources (default: 0.5 vCPU, 1.0 GB memory, 2 tasks)
- **database**: RDS instance type and storage (default: db.t3.small, 50 GB storage)
- **assets**: S3 storage for static assets (default: 20 GB S3 storage)
- **cdn**: CloudFront data transfer out (default: 200 GB data transfer)
- **loadBalancer**: Application Load Balancer capacity (default: 1460 LCU-hours)
- **natGateway**: NAT Gateway data processing (default: 100 GB data processed)
- **cloudwatchLogs**: CloudWatch Logs ingestion and storage (default: 10 GB logs)
- **availability**: Availability zone configuration (default: Single AZ)
- **backupRetention**: Database backup retention period (default: 7 days)

### Microservices

- **services**: Number and size of microservices (default: 3 services, 0.5 vCPU, 1.0 GB memory each)
- **tasksPerService**: Fargate tasks per microservice (default: 2 tasks per service)
- **database**: RDS instance type and storage (default: db.t3.medium, 100 GB storage)
- **storage**: S3 storage for assets and data (default: 50 GB S3 storage)
- **cdn**: CloudFront data transfer out (default: 300 GB data transfer)
- **loadBalancer**: Application Load Balancer capacity (default: 2190 LCU-hours)
- **apiGateway**: API Gateway requests per month (default: 1,000,000 requests)
- **messaging**: SQS message queue requests (default: 1,000,000 SQS requests)
- **cache**: ElastiCache Redis/Memcached nodes (default: 1 ElastiCache node)
- **natGateway**: NAT Gateway data processing (default: 200 GB data processed)
- **cloudwatchLogs**: CloudWatch Logs ingestion and storage (default: 20 GB logs)
- **availability**: Availability zone configuration (default: Single AZ)

## API Integration

### Example API Endpoint

```go
func GetBlueprintAssumptions(c echo.Context) error {
    blueprintType := c.Param("blueprintType")
    
    manager := NewUsageAssumptionsManager()
    assumptions, err := manager.GetAssumptions(blueprintType)
    if err != nil {
        return c.JSON(http.StatusNotFound, map[string]string{
            "error": err.Error(),
        })
    }
    
    return c.JSON(http.StatusOK, assumptions)
}
```

### Example Response

```json
{
  "blueprintType": "static-website",
  "assumptions": [
    {
      "key": "storage",
      "value": "10 GB",
      "description": "S3 storage for static assets",
      "unit": "GB"
    },
    {
      "key": "requests",
      "value": "10,000 per month",
      "description": "HTTP/HTTPS requests to website",
      "unit": "requests/month"
    }
  ],
  "asMap": {
    "storage": "10 GB",
    "requests": "10,000 per month"
  }
}
```

## Testing

The system includes comprehensive tests:

- **Unit Tests**: Test individual manager functions
- **Integration Tests**: Test integration with BlueprintMapper
- **Documentation Tests**: Verify assumptions are well-documented
- **Consistency Tests**: Ensure list and map representations match

Run tests:
```bash
go test ./pkg/cost/ -run "UsageAssumptions" -v
```

## Benefits

1. **Transparency**: Users can see exactly what assumptions are being made
2. **Configurability**: Assumptions can be customized per deployment
3. **Maintainability**: Centralized management makes updates easier
4. **Documentation**: Each assumption is self-documenting with descriptions
5. **Validation**: Built-in validation prevents invalid configurations
6. **Consistency**: Ensures assumptions are consistent across the system

## Future Enhancements

- **User Preferences**: Allow users to save custom assumption profiles
- **Historical Tracking**: Track how assumptions change over time
- **Recommendation Engine**: Suggest optimal assumptions based on actual usage
- **Regional Variations**: Support region-specific default assumptions
- **Cost Impact Analysis**: Show how changing assumptions affects cost estimates

## Validation Against Requirements

This implementation validates:

- **AC-5.4**: Shows assumptions (e.g., "Based on 10GB data transfer/month") ✓
- **TR-2.3**: Use realistic usage assumptions (documented and configurable) ✓

The system provides a robust, transparent, and configurable way to manage usage assumptions for AWS cost estimation.
