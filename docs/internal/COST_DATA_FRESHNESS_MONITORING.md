# Cost Data Freshness Monitoring

## Overview

The Cost Data Freshness Monitoring system tracks when actual AWS costs were last updated for active deployments and alerts operators when cost data becomes stale. This ensures that users always have up-to-date cost information and helps identify issues with the cost tracking pipeline.

## Features

### Automated Monitoring
- **Scheduled Job**: Runs every 6 hours to check cost data freshness
- **Active Deployment Tracking**: Monitors all active deployments that are old enough to have cost data (>48 hours)
- **Comprehensive Metrics**: Tracks fresh data, stale data, and missing data across all deployments

### Alerting System
- **Stale Data Alerts**: Automatically creates alerts when cost data is older than 48 hours
- **Missing Data Alerts**: Alerts when deployments have no cost data despite being old enough
- **Alert Deduplication**: Prevents duplicate alerts within 24 hours
- **User Notifications**: Alerts are stored in the `costAlerts` collection for user visibility

### Metrics and Reporting
- **Freshness Percentage**: Overall health metric showing percentage of deployments with fresh data
- **Data Age Statistics**: Tracks average and oldest data age across all deployments
- **Deployment Categorization**: Classifies deployments as fresh, stale, or missing data

## Architecture

### Components

#### 1. Cost Freshness Monitor Job
**File**: `pocketbase/pkg/jobs/cost_freshness_monitor.go`

The main monitoring job that:
- Queries all active deployments
- Checks the age of their cost data
- Generates freshness metrics
- Creates alerts for stale or missing data

**Schedule**: Every 6 hours (configurable)

#### 2. Freshness Metrics
```go
type CostFreshnessMetrics struct {
    TotalActiveDeployments    int
    DeploymentsWithFreshData  int
    DeploymentsWithStaleData  int
    DeploymentsWithoutData    int
    OldestDataAge             time.Duration
    AverageDataAge            time.Duration
    StaleDeployments          []StaleDeploymentInfo
    CheckedAt                 time.Time
}
```

#### 3. Stale Deployment Info
```go
type StaleDeploymentInfo struct {
    DeploymentID   string
    DeploymentName string
    LastFetchedAt  time.Time
    DataAge        time.Duration
    Status         string  // "stale" or "no_data"
    CreatedAt      time.Time
}
```

## Configuration

### Thresholds

```go
const (
    // MaxCostDataAge is the maximum acceptable age for cost data
    MaxCostDataAge = 48 * time.Hour
    
    // CostDataWarningAge is when we start warning about aging data
    CostDataWarningAge = 36 * time.Hour
)
```

### Job Schedule

The monitoring job is scheduled in `pricing_scheduler.go`:

```go
costFreshnessConfig := JobConfig{
    Name:        "cost-freshness-monitor",
    Schedule:    "0 0 */6 * * *", // Every 6 hours
    Enabled:     true,
    Timeout:     10 * time.Minute,
    MaxRetries:  2,
    Description: "Monitor cost data freshness and alert on stale data",
}
```

## Usage

### Programmatic Access

#### Check Overall Freshness Status
```go
manager := jobs.NewPricingJobManager(app)
metrics, err := manager.GetCostDataFreshnessStatus()
if err != nil {
    log.Printf("Failed to get freshness status: %v", err)
}

log.Printf("Fresh data: %d/%d deployments (%.1f%%)",
    metrics.DeploymentsWithFreshData,
    metrics.TotalActiveDeployments,
    float64(metrics.DeploymentsWithFreshData)/float64(metrics.TotalActiveDeployments)*100)
```

#### Check Specific Deployment
```go
manager := jobs.NewPricingJobManager(app)
isFresh, dataAge, err := manager.IsCostDataFresh(deploymentID)
if err != nil {
    log.Printf("No cost data found: %v", err)
} else if !isFresh {
    log.Printf("Cost data is stale: %v old", dataAge)
}
```

#### Get List of Stale Deployments
```go
manager := jobs.NewPricingJobManager(app)
staleDeployments, err := manager.GetStaleDeployments()
if err != nil {
    log.Printf("Failed to get stale deployments: %v", err)
}

for _, deployment := range staleDeployments {
    log.Printf("Stale: %s (%s) - last fetched %v ago",
        deployment.DeploymentName,
        deployment.DeploymentID,
        deployment.DataAge)
}
```

#### Manual Trigger
```go
manager := jobs.NewPricingJobManager(app)
err := manager.TriggerCostFreshnessMonitor()
if err != nil {
    log.Printf("Failed to trigger freshness monitor: %v", err)
}
```

### API Integration

The freshness monitoring can be exposed via API endpoints:

```go
// GET /api/admin/cost/freshness
func GetCostFreshnessStatus(c echo.Context) error {
    manager := jobs.NewPricingJobManager(app)
    metrics, err := manager.GetCostDataFreshnessStatus()
    if err != nil {
        return c.JSON(500, map[string]string{"error": err.Error()})
    }
    return c.JSON(200, metrics)
}

// GET /api/admin/cost/stale-deployments
func GetStaleDeployments(c echo.Context) error {
    manager := jobs.NewPricingJobManager(app)
    staleDeployments, err := manager.GetStaleDeployments()
    if err != nil {
        return c.JSON(500, map[string]string{"error": err.Error()})
    }
    return c.JSON(200, staleDeployments)
}

// POST /api/admin/cost/check-freshness
func TriggerFreshnessCheck(c echo.Context) error {
    manager := jobs.NewPricingJobManager(app)
    err := manager.TriggerCostFreshnessMonitor()
    if err != nil {
        return c.JSON(500, map[string]string{"error": err.Error()})
    }
    return c.JSON(200, map[string]string{"status": "triggered"})
}
```

## Alert Management

### Alert Structure

Alerts are stored in the `costAlerts` collection with the following fields:

```javascript
{
  "deployment": "deployment_id",
  "user": "user_id",
  "type": "cost_data_stale",
  "threshold": 0.0,  // Not applicable for freshness alerts
  "triggered": true,
  "actualCost": 0.0,
  "estimatedCost": 0.0,
  "variance": 0.0,
  "message": "Cost data for deployment 'my-app' is stale. Last fetched 60h ago (threshold: 48h).",
  "sentAt": "2026-04-11T10:00:00Z",
  "acknowledged": false
}
```

### Alert Types

1. **Stale Data Alert**: Cost data exists but is older than 48 hours
   - Status: `"stale"`
   - Message includes last fetch time and data age

2. **Missing Data Alert**: No cost data exists for deployment old enough to have it
   - Status: `"no_data"`
   - Message indicates deployment age and missing data

### Alert Deduplication

The system prevents alert spam by:
- Checking for existing unacknowledged alerts
- Only creating new alerts if no recent alert exists (within 24 hours)
- Allowing users to acknowledge alerts to clear them

## Monitoring and Observability

### Metrics to Track

1. **Freshness Percentage**: `deploymentsWithFreshData / totalActiveDeployments * 100`
   - Target: >95%
   - Warning: <90%
   - Critical: <80%

2. **Average Data Age**: Mean age of all cost data
   - Target: <24 hours
   - Warning: >30 hours
   - Critical: >42 hours

3. **Stale Deployment Count**: Number of deployments with stale data
   - Target: 0
   - Warning: >2
   - Critical: >5

4. **Missing Data Count**: Deployments without any cost data
   - Target: 0
   - Warning: >1
   - Critical: >3

### Log Messages

The monitoring job logs detailed information:

```
Cost Data Freshness Metrics:
  Total Active Deployments: 10
  Deployments with Fresh Data: 8
  Deployments with Stale Data: 1
  Deployments without Data: 1
  Average Data Age: 18h30m
  Oldest Data Age: 52h15m
  Data Freshness: 80.0%
```

### Health Checks

Include freshness metrics in system health checks:

```go
func CheckCostSystemHealth() (bool, []string) {
    var issues []string
    
    manager := jobs.NewPricingJobManager(app)
    metrics, err := manager.GetCostDataFreshnessStatus()
    if err != nil {
        issues = append(issues, fmt.Sprintf("Cannot check cost freshness: %v", err))
        return false, issues
    }
    
    // Check freshness percentage
    if metrics.TotalActiveDeployments > 0 {
        freshnessPercentage := float64(metrics.DeploymentsWithFreshData) / 
                               float64(metrics.TotalActiveDeployments) * 100
        if freshnessPercentage < 80 {
            issues = append(issues, 
                fmt.Sprintf("Cost data freshness is low: %.1f%%", freshnessPercentage))
        }
    }
    
    // Check for stale data
    if metrics.DeploymentsWithStaleData > 5 {
        issues = append(issues, 
            fmt.Sprintf("%d deployments have stale cost data", 
                metrics.DeploymentsWithStaleData))
    }
    
    return len(issues) == 0, issues
}
```

## Troubleshooting

### Issue: High Number of Stale Deployments

**Symptoms**: Many deployments showing stale cost data

**Possible Causes**:
1. Actual cost fetch job is failing
2. AWS Cost Explorer API issues
3. Circuit breaker is open
4. Rate limiting from AWS

**Resolution**:
1. Check actual cost fetch job logs
2. Verify AWS credentials and permissions
3. Check circuit breaker status
4. Review AWS API rate limits

### Issue: Missing Cost Data

**Symptoms**: Deployments have no cost data despite being old enough

**Possible Causes**:
1. Deployment tags not properly set
2. Cost Explorer filter not matching
3. Deployment created before cost tracking was enabled
4. AWS Cost Explorer delay (48 hours)

**Resolution**:
1. Verify deployment has correct tags
2. Check Cost Explorer filter configuration
3. Manually trigger cost fetch for specific deployment
4. Wait for AWS Cost Explorer delay period

### Issue: False Positive Alerts

**Symptoms**: Alerts for deployments that should have fresh data

**Possible Causes**:
1. Clock skew between systems
2. Timezone issues
3. Recent deployment that's not old enough yet

**Resolution**:
1. Verify system time synchronization
2. Check timezone configuration
3. Review deployment age calculation logic

## Best Practices

### 1. Regular Monitoring
- Review freshness metrics daily
- Set up automated alerts for low freshness percentage
- Track trends over time

### 2. Alert Response
- Investigate stale data alerts within 4 hours
- Acknowledge alerts after resolution
- Document recurring issues

### 3. Maintenance
- Review and adjust thresholds based on operational experience
- Update alert messages to be more actionable
- Integrate with existing monitoring systems (Prometheus, Datadog, etc.)

### 4. Testing
- Run integration tests regularly
- Test alert generation in staging environment
- Verify alert deduplication works correctly

## Future Enhancements

### Planned Features
1. **Configurable Thresholds**: Per-deployment or per-blueprint thresholds
2. **Email Notifications**: Send email alerts to deployment owners
3. **Slack Integration**: Post alerts to Slack channels
4. **Metrics Dashboard**: Visual dashboard for freshness metrics
5. **Automatic Remediation**: Trigger cost fetch when data becomes stale
6. **Historical Tracking**: Store freshness metrics over time for trend analysis

### Integration Opportunities
1. **Prometheus Metrics**: Export freshness metrics for Prometheus
2. **Grafana Dashboards**: Pre-built dashboards for cost freshness
3. **PagerDuty**: Escalate critical freshness issues
4. **AWS CloudWatch**: Publish metrics to CloudWatch

## Related Documentation

- [Cost Explorer Error Handling](./COST_EXPLORER_ERROR_HANDLING.md)
- [Cost Cleanup Job](./COST_CLEANUP_JOB.md)
- [AWS Cost Estimation API](./AWS_COST_ESTIMATION_API.md)
- [Project Status Verification](./PROJECT_STATUS_VERIFICATION.md)

## Support

For issues or questions about cost data freshness monitoring:
1. Check the logs for detailed error messages
2. Review the troubleshooting section above
3. Consult the related documentation
4. Contact the platform team for assistance
