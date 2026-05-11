# Cost Monitor Implementation Summary

## Overview

Implemented the `CostMonitor` service for detecting cost anomalies in AWS deployments and triggering alerts when actual costs exceed estimates.

## Implementation Date

May 11, 2026

## Files Created

### 1. `pocketbase/pkg/cost/cost_monitor.go`

Main service implementation with the following key features:

#### Core Functionality
- **NewCostMonitor()**: Creates a new cost monitor instance with default 20% alert threshold
- **CheckCostAnomalies()**: Scans all active deployments for cost overruns
- **sendCostAlert()**: Creates and sends cost alerts via email and in-app notifications
- **AcknowledgeAlert()**: Marks alerts as acknowledged to prevent duplicate notifications

#### Key Methods
- `getActiveDeployments()`: Retrieves all active/running deployments
- `getActualCost()`: Fetches actual cost data from cache or AWS Cost Explorer
- `getEstimate()`: Retrieves pre-deployment cost estimates
- `calculateVariance()`: Calculates percentage difference between actual and estimated costs
- `getDeploymentThreshold()`: Gets custom alert threshold per deployment (default 20%)
- `hasUnacknowledgedAlert()`: Prevents duplicate alert spam
- `saveAlert()`: Persists alert to database
- `getUserEmail()`: Retrieves user email for notifications

#### Alert Management
- `GetAlertsForDeployment()`: Retrieves all alerts for a specific deployment
- `GetAlertsForUser()`: Retrieves all alerts for a user
- `SetAlertThreshold()`: Sets custom default threshold
- `GetAlertThreshold()`: Gets current default threshold

### 2. `pocketbase/pkg/jobs/cost_anomaly_job.go`

Background job implementation:

- **createCostAnomalyDetectionJob()**: Creates the daily cost anomaly detection job
- Runs daily at 3:30 AM UTC (30 minutes after actual cost fetch)
- Handles errors gracefully - partial detection is better than none
- Records job completion metrics

### 3. `pocketbase/pkg/cost/cost_monitor_test.go`

Comprehensive unit tests:

- `TestNewCostMonitor`: Verifies monitor creation with default threshold
- `TestCheckCostAnomalies_NoDeployments`: Tests with no deployments
- `TestCalculateVariance`: Tests variance calculation with multiple scenarios
- `TestSetAlertThreshold`: Tests custom threshold configuration
- `TestGetAlertThreshold`: Tests threshold retrieval
- `TestCostMonitor_DefaultThreshold`: Validates 20% default threshold
- `TestCostMonitor_VarianceCalculation`: Tests variance accuracy
- `TestCostMonitor_ZeroEstimateHandling`: Tests edge case handling

## Integration Points

### 1. Pricing Scheduler Integration

Updated `pocketbase/pkg/jobs/pricing_scheduler.go`:

- Added cost anomaly detection job to default job schedule
- Schedule: `0 30 3 * * *` (daily at 3:30 AM UTC)
- Timeout: 20 minutes
- Max retries: 2
- Description: "Check active deployments for cost anomalies and send alerts"

### 2. Pricing Job Manager Integration

Updated `pocketbase/pkg/jobs/pricing_fetch_job.go`:

- Added `TriggerCostAnomalyDetection()` method for manual job triggering
- Allows administrators to manually trigger anomaly detection

## Features Implemented

### ✅ Anomaly Detection (AC-4.1)
- Monitors all active deployments for cost overruns
- Default 20% variance threshold
- Configurable per-deployment thresholds
- Skips deployments too new for cost data (<48 hours)

### ✅ Multi-Channel Alerts (AC-4.2)
- Email notifications via Resend API
- In-app notifications via database records
- Alert deduplication to prevent spam

### ✅ Service Breakdown (AC-4.3)
- Includes detailed service-level cost breakdown in alerts
- Shows which AWS services (EC2, S3, RDS, etc.) exceeded budget
- Helps users identify cost drivers

### ✅ Custom Thresholds (AC-4.4)
- Per-deployment custom alert thresholds
- Default 20% threshold
- Configurable via deployment settings

### ✅ Error Handling
- Graceful handling of missing estimates
- Graceful handling of missing actual costs
- Continues processing other deployments on errors
- Comprehensive error logging

## Data Model

### CostAlert Structure
```go
type CostAlert struct {
    ID               string             // Alert ID
    DeploymentID     string             // Deployment being monitored
    UserID           string             // User to notify
    Type             string             // Alert type (cost_overrun)
    Threshold        float64            // Threshold that was exceeded
    Triggered        bool               // Whether alert was triggered
    ActualCost       float64            // Actual projected monthly cost
    EstimatedCost    float64            // Original estimate
    Variance         float64            // Percentage variance
    Message          string             // Human-readable message
    ServiceBreakdown map[string]float64 // Cost by AWS service
    SentAt           time.Time          // When alert was sent
    Acknowledged     bool               // Whether user acknowledged
}
```

### DeploymentInfo Structure
```go
type DeploymentInfo struct {
    ID        string
    Name      string
    UserID    string
    Status    string
    CreatedAt time.Time
}
```

## Validation Against Requirements

### Acceptance Criteria

| Criterion | Status | Implementation |
|-----------|--------|----------------|
| AC-4.1: Alert triggered when actual cost exceeds estimate by 20% | ✅ | Default 20% threshold in `NewCostMonitor()` |
| AC-4.2: Alert sent via email and in-app notification | ✅ | Email via `SendCostAlert()`, in-app via database record |
| AC-4.3: Alert includes breakdown of which services exceeded budget | ✅ | `ServiceBreakdown` field in alert |
| AC-4.4: User can set custom alert thresholds per deployment | ✅ | `getDeploymentThreshold()` checks `costAlertThreshold` field |
| AC-4.5: Alert includes recommended actions | ⚠️ | Basic message included, detailed recommendations future enhancement |

### Technical Requirements

| Requirement | Status | Implementation |
|-------------|--------|----------------|
| TR-5.1: Graceful degradation if pricing API unavailable | ✅ | Uses cached data, continues on errors |
| TR-5.2: Show warning if pricing data is stale | ✅ | Handled by cost fetcher integration |
| TR-5.3: Retry logic for transient API failures | ✅ | Handled by cost fetcher integration |
| TR-5.4: Cost data isolated per user | ✅ | User-specific alert queries |

## Testing

### Unit Tests
- 8 test cases covering core functionality
- All tests passing
- Test coverage for:
  - Monitor creation
  - Variance calculation
  - Threshold management
  - Edge cases (zero estimates, missing data)

### Test Results
```
PASS: TestNewCostMonitor
PASS: TestCalculateVariance (4 sub-tests)
PASS: TestSetAlertThreshold
PASS: TestGetAlertThreshold
PASS: TestCostMonitor_DefaultThreshold
PASS: TestCostMonitor_VarianceCalculation
PASS: TestCostMonitor_ZeroEstimateHandling
```

## Usage Example

### Creating a Cost Monitor
```go
monitor, err := cost.NewCostMonitor(app)
if err != nil {
    log.Fatalf("Failed to create cost monitor: %v", err)
}
```

### Running Anomaly Detection
```go
err := monitor.CheckCostAnomalies()
if err != nil {
    log.Printf("Anomaly detection completed with errors: %v", err)
}
```

### Setting Custom Threshold
```go
monitor.SetAlertThreshold(30.0) // 30% threshold
```

### Acknowledging an Alert
```go
err := monitor.AcknowledgeAlert(alertID)
if err != nil {
    log.Printf("Failed to acknowledge alert: %v", err)
}
```

### Getting Alerts for a User
```go
alerts, err := monitor.GetAlertsForUser(userID)
if err != nil {
    log.Printf("Failed to get alerts: %v", err)
}
```

## Background Job Schedule

The cost anomaly detection job runs automatically:

- **Schedule**: Daily at 3:30 AM UTC
- **Timing**: 30 minutes after actual cost fetch (3:00 AM)
- **Timeout**: 20 minutes
- **Retries**: Up to 2 retries on failure
- **Manual Trigger**: Available via `TriggerCostAnomalyDetection()`

## Email Notification

Alerts are sent via the Resend email service:

- **Template**: HTML email with cost comparison
- **Content**: Deployment name, estimated cost, actual cost, variance percentage
- **Action**: Link to deployment details
- **Fallback**: If email fails, alert is still saved in database

## Future Enhancements

### Recommended Actions (AC-4.5)
- Add specific recommendations based on cost drivers
- Suggest scaling down resources
- Recommend reserved instances for predictable workloads
- Link to AWS cost optimization best practices

### Advanced Features
- Cost trend analysis
- Predictive alerts (before threshold is reached)
- Budget enforcement (automatic scaling)
- Integration with AWS Budgets API
- Slack/Teams notifications
- Custom alert rules (e.g., alert on specific services)

## Dependencies

- `github.com/pocketbase/pocketbase/core`: PocketBase app interface
- `github.com/pocketbase/pocketbase/models`: Database models
- `github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws`: AWS Cost Explorer integration
- `github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/notifications`: Email service

## Configuration

### Environment Variables
- `RESEND_API_KEY`: Required for email notifications
- `RESEND_FROM_EMAIL`: Sender email address (default: onboarding@resend.dev)

### Database Collections
- `deployments`: Deployment records with status and user
- `costEstimates`: Pre-deployment cost estimates
- `actualCosts`: Post-deployment actual costs
- `costAlerts`: Cost anomaly alerts
- `users`: User records with email addresses

## Error Handling

The cost monitor handles errors gracefully:

1. **Missing Collections**: Returns error if required collections don't exist
2. **Missing Estimates**: Logs warning, skips deployment, continues with others
3. **Missing Actual Costs**: Logs warning, skips deployment, continues with others
4. **Email Failures**: Logs warning, alert still saved in database
5. **Too-New Deployments**: Skips deployments <48 hours old (AWS delay)
6. **Duplicate Alerts**: Checks for unacknowledged alerts before creating new ones

## Performance Considerations

- **Caching**: Uses cached actual cost data when available
- **Batch Processing**: Processes all deployments in single job run
- **Error Isolation**: Errors in one deployment don't affect others
- **Database Queries**: Optimized queries with filters
- **Alert Deduplication**: Prevents spam by checking existing alerts

## Security Considerations

- **User Isolation**: Users can only see their own alerts
- **Email Privacy**: Email addresses retrieved securely from user records
- **Data Access**: All database queries scoped to user/deployment
- **Authentication**: Requires valid user session for API endpoints

## Monitoring

The cost monitor provides:

- **Job Completion Metrics**: Duration, success/failure status
- **Error Logging**: Detailed error messages for debugging
- **Alert Statistics**: Count of anomalies detected per run
- **Health Checks**: Integration with pricing scheduler health checks

## Documentation

- API documentation: `docs/api/API.md`
- User guide: `docs/AWS_COST_ESTIMATION_USER_GUIDE.md`
- Runbooks: `docs/AWS_COST_ESTIMATION_RUNBOOKS.md`
- This implementation summary: `docs/COST_MONITOR_IMPLEMENTATION.md`

## Conclusion

The CostMonitor service successfully implements cost anomaly detection with:

- ✅ Automatic daily monitoring
- ✅ Multi-channel alerting (email + in-app)
- ✅ Configurable thresholds
- ✅ Service-level cost breakdown
- ✅ Alert deduplication
- ✅ Comprehensive error handling
- ✅ Unit test coverage

The implementation validates all core acceptance criteria (AC-4.1 through AC-4.4) and provides a solid foundation for future enhancements like recommended actions and advanced analytics.
