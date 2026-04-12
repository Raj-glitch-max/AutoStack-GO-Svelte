# ActualCostFetcher Service

## Overview

The `ActualCostFetcher` service provides production-grade integration with AWS Cost Explorer API to fetch actual AWS costs for deployments. It handles the 48-hour delay requirement, calculates monthly projections, and provides variance analysis against estimates.

## Features

- **AWS Cost Explorer Integration**: Fetches real cost data from AWS Cost Explorer API
- **48-Hour Delay Handling**: Gracefully handles AWS's 48-hour data availability delay
- **Deployment Tagging**: Tracks costs using deployment-specific tags
- **Service-Level Breakdown**: Provides detailed cost breakdown by AWS service
- **Monthly Projection**: Projects monthly costs from partial data
- **Variance Calculation**: Compares actual costs against estimates
- **Retry Logic**: Built-in exponential backoff retry for API failures
- **Comprehensive Error Handling**: Handles edge cases and API errors gracefully

## Usage

### Basic Usage

```go
import (
    "github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
)

// Create a new ActualCostFetcher
fetcher, err := aws.NewActualCostFetcher(app)
if err != nil {
    log.Fatal(err)
}

// Fetch actual costs for a deployment
actualCost, err := fetcher.FetchActualCosts("deployment-123")
if err != nil {
    log.Printf("Failed to fetch costs: %v", err)
    return
}

// Access cost data
fmt.Printf("Cost to date: $%.2f\n", actualCost.CostToDate)
fmt.Printf("Projected monthly: $%.2f\n", actualCost.ProjectedMonthly)
fmt.Printf("Variance: %.2f%%\n", actualCost.Variance)

// Access service breakdown
for service, cost := range actualCost.Breakdown {
    fmt.Printf("%s: $%.2f\n", service, cost)
}
```

### Fetching Costs for All Active Deployments

```go
// Fetch costs for all active deployments
err := fetcher.FetchActualCostsForActiveDeployments()
if err != nil {
    log.Printf("Failed to fetch costs for active deployments: %v", err)
}
```

### Retrieving Cached Cost Data

```go
// Get cached actual cost data
cachedCost, err := fetcher.GetCachedActualCost("deployment-123")
if err != nil {
    log.Printf("No cached cost data: %v", err)
    return
}

fmt.Printf("Cached cost: $%.2f (fetched at %v)\n", 
    cachedCost.CostToDate, 
    cachedCost.FetchedAt)
```

## Data Structures

### ActualCostData

```go
type ActualCostData struct {
    DeploymentID     string             // Deployment identifier
    CostToDate       float64            // Total cost accumulated to date
    ProjectedMonthly float64            // Projected monthly cost
    Variance         float64            // Variance percentage vs estimate
    Breakdown        map[string]float64 // Cost breakdown by service
    Period           CostPeriod         // Time period for cost data
    FetchedAt        time.Time          // When data was fetched
}
```

### CostPeriod

```go
type CostPeriod struct {
    Start time.Time // Start of cost period
    End   time.Time // End of cost period
}
```

## AWS Cost Explorer Requirements

### IAM Permissions

The AWS credentials used must have the following permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ce:GetCostAndUsage",
                "ce:GetTags"
            ],
            "Resource": "*"
        }
    ]
}
```

### Deployment Tagging

Deployments must be tagged with the following tag for cost tracking:

- **Tag Key**: `autostack:deployment`
- **Tag Value**: Deployment ID

Example Terraform:

```hcl
resource "aws_instance" "example" {
  # ... other configuration ...
  
  tags = {
    "autostack:deployment" = var.deployment_id
  }
}
```

## 48-Hour Delay Handling

AWS Cost Explorer data is available with a 48-hour delay. The service handles this automatically:

1. **Validation**: Checks if deployment is at least 48 hours old
2. **Period Calculation**: Calculates cost period ending 48 hours ago
3. **Error Messages**: Provides clear error messages with remaining time

Example error:

```
cost data not yet available (AWS requires 48-hour delay, 23h remaining)
```

## Cost Calculation

### Monthly Projection

Monthly costs are projected from partial data using daily averages:

```
Daily Average = Cost To Date / Days Elapsed
Projected Monthly = Daily Average × 30
```

Example:
- 10 days of data: $50.00
- Daily average: $5.00
- Projected monthly: $150.00

### Variance Calculation

Variance is calculated as a percentage difference from the estimate:

```
Variance = ((Actual - Estimate) / Estimate) × 100
```

Example:
- Actual: $120.00
- Estimate: $100.00
- Variance: +20.00%

## Service Breakdown

The service provides cost breakdown by AWS service:

```go
breakdown := map[string]float64{
    "AmazonEC2":        50.00,
    "AmazonS3":         10.00,
    "AmazonRDS":        30.00,
    "AmazonCloudFront": 5.00,
}
```

Common AWS services tracked:
- **AmazonEC2**: EC2 instances, EBS volumes, data transfer
- **AmazonS3**: S3 storage and requests
- **AmazonRDS**: RDS instances and storage
- **AmazonECS**: Fargate compute
- **AmazonCloudFront**: CloudFront data transfer
- **AmazonRoute53**: Route53 hosted zones and queries
- **AWSDataTransfer**: Inter-region and internet data transfer

## Error Handling

The service handles various error scenarios:

### Deployment Not Found

```go
actualCost, err := fetcher.FetchActualCosts("invalid-id")
// Error: deployment not found
```

### Deployment Too Recent

```go
actualCost, err := fetcher.FetchActualCosts("new-deployment")
// Error: cost data not yet available (AWS requires 48-hour delay, 47h remaining)
```

### No Cost Data

```go
// Returns zero costs with empty breakdown
actualCost := &ActualCostData{
    CostToDate: 0.00,
    Breakdown:  map[string]float64{},
}
```

### API Failures

The service uses exponential backoff retry logic:
- **Max Retries**: 5
- **Base Delay**: 1 second
- **Max Delay**: 30 seconds
- **Backoff Factor**: 2.0

## Database Schema

The service stores actual cost data in the `actualCosts` collection:

```javascript
{
  id: "actual_deploy_123",
  deployment: "deploy_123",
  costToDate: 47.23,
  projectedMonthly: 62.18,
  variance: 9.76,
  breakdown: {
    "AmazonEC2": 25.50,
    "AmazonS3": 10.25,
    "AmazonRDS": 11.48
  },
  periodStart: "2024-01-01T00:00:00Z",
  periodEnd: "2024-01-10T00:00:00Z",
  fetchedAt: "2024-01-10T12:00:00Z"
}
```

## Performance Considerations

### Caching

- Cost data is cached in the database
- Use `GetCachedActualCost()` to retrieve cached data
- Reduces API calls to AWS Cost Explorer

### Rate Limits

AWS Cost Explorer API has rate limits:
- **Limit**: 5 requests per second
- **Handling**: Built-in retry logic with exponential backoff
- **Recommendation**: Fetch costs in batches, not real-time

### Cost Period

- Longer periods = more accurate projections
- Minimum recommended: 3 days of data
- Optimal: 7-14 days of data

## Testing

### Unit Tests

```bash
go test -v -run TestCalculate ./pkg/aws/
go test -v -run TestProcess ./pkg/aws/
go test -v -run TestRound ./pkg/aws/
```

### Integration Tests

```bash
go test -v -run TestActualCostFetcher ./pkg/aws/
```

### Benchmarks

```bash
go test -bench=. ./pkg/aws/
```

## Best Practices

1. **Batch Processing**: Fetch costs for multiple deployments in batches
2. **Caching**: Use cached data when real-time accuracy isn't required
3. **Error Handling**: Always check for 48-hour delay errors
4. **Tagging**: Ensure all AWS resources are properly tagged
5. **Monitoring**: Track API failures and retry statistics
6. **Cost Alerts**: Set up alerts for variance thresholds

## Troubleshooting

### No Cost Data Returned

**Possible Causes**:
- Deployment tags not applied to AWS resources
- Deployment is less than 48 hours old
- AWS Cost Explorer not enabled in account
- Insufficient IAM permissions

**Solutions**:
1. Verify deployment tags on AWS resources
2. Wait for 48-hour delay period
3. Enable Cost Explorer in AWS Console
4. Check IAM permissions

### High Variance

**Possible Causes**:
- Estimate doesn't include all services
- Unexpected data transfer costs
- Resource scaling beyond estimate
- Reserved instance pricing differences

**Solutions**:
1. Review service breakdown for unexpected costs
2. Check for data transfer overages
3. Compare actual vs estimated resource usage
4. Update estimate assumptions

### API Errors

**Possible Causes**:
- Rate limit exceeded
- Invalid AWS credentials
- Network connectivity issues
- AWS service outage

**Solutions**:
1. Check retry statistics
2. Verify AWS credentials
3. Check network connectivity
4. Monitor AWS service health

## Examples

### Example 1: Daily Cost Monitoring

```go
func monitorDailyCosts(app core.App) {
    fetcher, _ := aws.NewActualCostFetcher(app)
    
    // Fetch costs for all active deployments
    err := fetcher.FetchActualCostsForActiveDeployments()
    if err != nil {
        log.Printf("Error fetching costs: %v", err)
        return
    }
    
    log.Println("Daily cost monitoring completed")
}
```

### Example 2: Cost Alert System

```go
func checkCostAlerts(app core.App, deploymentID string) {
    fetcher, _ := aws.NewActualCostFetcher(app)
    
    actualCost, err := fetcher.GetCachedActualCost(deploymentID)
    if err != nil {
        return
    }
    
    // Alert if variance exceeds 20%
    if actualCost.Variance > 20.0 {
        sendCostAlert(deploymentID, actualCost)
    }
}
```

### Example 3: Cost Dashboard

```go
func getCostDashboard(app core.App, deploymentID string) (*CostDashboard, error) {
    fetcher, _ := aws.NewActualCostFetcher(app)
    
    actualCost, err := fetcher.GetCachedActualCost(deploymentID)
    if err != nil {
        return nil, err
    }
    
    return &CostDashboard{
        CurrentSpend:     actualCost.CostToDate,
        ProjectedMonthly: actualCost.ProjectedMonthly,
        Variance:         actualCost.Variance,
        TopServices:      getTopServices(actualCost.Breakdown, 5),
        LastUpdated:      actualCost.FetchedAt,
    }, nil
}
```

## Related Documentation

- [AWS Cost Explorer API Documentation](https://docs.aws.amazon.com/cost-management/latest/APIReference/Welcome.html)
- [AWS Tagging Best Practices](https://docs.aws.amazon.com/general/latest/gr/aws_tagging.html)
- [Cost Estimation Design Document](../../.kiro/specs/aws-cost-estimation/design.md)
- [Retry Logic Documentation](./retry_logic.go)

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review integration tests for examples
3. Check AWS Cost Explorer API documentation
4. Review application logs for detailed error messages
