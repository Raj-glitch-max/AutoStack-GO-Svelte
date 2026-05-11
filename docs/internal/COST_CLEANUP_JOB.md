# Cost Data Cleanup Job

## Overview

The Cost Data Cleanup Job is a background job that automatically removes cost-related data for destroyed or failed AWS deployments. This helps maintain database hygiene and prevents accumulation of stale cost data.

## Purpose

When deployments are destroyed or fail, their associated cost data (estimates, actual costs, and alerts) should be cleaned up to:
- Reduce database storage usage
- Improve query performance
- Maintain data accuracy
- Comply with data retention policies

## Schedule

The cleanup job runs **daily at 4:00 AM UTC**, after the actual cost fetch job completes.

**Cron Expression**: `0 0 4 * * *`

## What Gets Cleaned Up

For each deployment with status `destroyed` or `failed`, the job removes:

1. **Actual Costs** (`actualCosts` collection)
   - Cost-to-date records
   - Projected monthly costs
   - Service-level cost breakdowns
   - Historical cost data

2. **Cost Estimates** (`costEstimates` collection)
   - Pre-deployment cost estimates
   - Cost ranges and assumptions
   - Pricing metadata

3. **Cost Alerts** (`costAlerts` collection)
   - Cost overrun alerts
   - Alert history
   - Notification records

## Behavior

### Normal Operation

1. Query for all deployments with status `destroyed` or `failed`
2. For each deployment:
   - Delete all records from `actualCosts` collection
   - Delete all records from `costEstimates` collection
   - Delete all records from `costAlerts` collection
3. Log the number of records deleted
4. Record cleanup metrics

### Edge Cases

- **No destroyed deployments**: Job completes successfully with no action
- **Missing collections**: Job skips collections that don't exist
- **Partial failures**: Job continues processing other deployments if one fails
- **Active deployments**: Job only processes `destroyed` or `failed` deployments

## Manual Triggering

You can manually trigger the cleanup job through the API or programmatically:

### Via API (Admin Only)

```bash
POST /api/admin/jobs/trigger
{
  "jobName": "daily-cost-cleanup"
}
```

### Programmatically

```go
manager := jobs.NewPricingJobManager(app)
err := manager.TriggerCostCleanup()
if err != nil {
    log.Printf("Failed to trigger cleanup: %v", err)
}
```

## Monitoring

### Job Status

Check the status of the cleanup job:

```go
manager := jobs.NewPricingJobManager(app)
statuses := manager.GetJobStatus()

for _, status := range statuses {
    if status.Name == "daily-cost-cleanup" {
        fmt.Printf("Last run: %v\n", status.LastRun)
        fmt.Printf("Next run: %v\n", status.NextRun)
        fmt.Printf("Success count: %d\n", status.SuccessCount)
        fmt.Printf("Error count: %d\n", status.ErrorCount)
    }
}
```

### Cost Data Statistics

Get statistics about cost data storage:

```go
scheduler := jobs.NewPricingScheduler(app)
stats, err := scheduler.GetCostDataStats()
if err != nil {
    log.Printf("Failed to get stats: %v", err)
}

fmt.Printf("Actual costs records: %d\n", stats["actualCosts"])
fmt.Printf("Cost estimates records: %d\n", stats["costEstimates"])
fmt.Printf("Cost alerts records: %d\n", stats["costAlerts"])
fmt.Printf("Destroyed deployments with cost data: %d\n", 
    stats["destroyedDeploymentsWithCostData"])
```

## Configuration

### Job Configuration

The cleanup job is configured in `pricing_scheduler.go`:

```go
costCleanupConfig := JobConfig{
    Name:        "daily-cost-cleanup",
    Schedule:    "0 0 4 * * *",  // Daily at 4 AM UTC
    Enabled:     true,
    Timeout:     15 * time.Minute,
    MaxRetries:  2,
    Description: "Clean up cost data for destroyed and failed deployments",
}
```

### Customization

To change the schedule or timeout:

1. Edit `pocketbase/pkg/jobs/pricing_scheduler.go`
2. Modify the `costCleanupConfig` in `scheduleDefaultJobs()`
3. Rebuild and restart the application

## Data Retention

### Default Behavior

By default, the job cleans up cost data for **all** destroyed/failed deployments immediately.

### Extended Retention

For compliance or auditing purposes, you can implement extended retention:

```go
// Clean up only deployments destroyed more than 90 days ago
scheduler := jobs.NewPricingScheduler(app)
err := scheduler.CleanupOldCostData(90) // 90 days retention
```

This can be scheduled as a separate weekly job if needed.

## Error Handling

### Retry Logic

- The job has a maximum of **2 retries** on failure
- Retries use exponential backoff
- Partial failures don't prevent processing other deployments

### Logging

All cleanup operations are logged:

```
2026-05-04 04:00:00 Starting cost data cleanup job for destroyed deployments...
2026-05-04 04:00:01 Found 5 destroyed/failed deployments to clean up
2026-05-04 04:00:01 Cleaned up 3 cost records for deployment abc123
2026-05-04 04:00:02 Cleaned up 2 cost records for deployment def456
2026-05-04 04:00:03 Cost cleanup job completed: deleted 15 records from 5 deployments in 3.2s (errors: 0)
```

### Error Scenarios

| Error | Behavior | Resolution |
|-------|----------|------------|
| Collection not found | Skip collection, continue | Ensure migrations are run |
| Record deletion fails | Log error, continue with next | Check database permissions |
| Timeout exceeded | Job terminates, retries later | Increase timeout if needed |
| Database connection lost | Job fails, retries with backoff | Check database health |

## Performance

### Expected Performance

- **Small deployments** (< 100 destroyed): < 1 second
- **Medium deployments** (100-1000 destroyed): 1-5 seconds
- **Large deployments** (> 1000 destroyed): 5-30 seconds

### Optimization

The job is optimized for:
- Batch deletion operations
- Minimal database queries
- Efficient filtering by deployment status

### Resource Usage

- **CPU**: Low (mostly I/O bound)
- **Memory**: Low (processes deployments sequentially)
- **Database**: Moderate (multiple DELETE operations)

## Testing

### Unit Tests

Run unit tests:

```bash
cd pocketbase
go test -v ./pkg/jobs/... -run TestCostCleanup
```

### Integration Tests

Run integration tests (requires database):

```bash
cd pocketbase
go test -v ./pkg/jobs/cost_cleanup_integration_test.go
```

### Manual Testing

1. Create a test deployment and mark it as destroyed
2. Create cost data for the deployment
3. Manually trigger the cleanup job
4. Verify cost data is deleted

```go
// Create test deployment
deployment.Set("status", "destroyed")
app.Dao().SaveRecord(deployment)

// Trigger cleanup
manager := jobs.NewPricingJobManager(app)
manager.TriggerCostCleanup()

// Verify deletion
record, err := app.Dao().FindFirstRecordByFilter(
    "actualCosts",
    "deployment = {:id}",
    map[string]any{"id": deployment.Id},
)
// Should return error (record not found)
```

## Troubleshooting

### Job Not Running

**Symptoms**: Cleanup job doesn't execute at scheduled time

**Possible Causes**:
1. Scheduler not started
2. Job disabled in configuration
3. System time incorrect

**Resolution**:
```go
// Check if scheduler is running
manager := jobs.NewPricingJobManager(app)
if !manager.scheduler.IsRunning() {
    log.Println("Scheduler is not running!")
    manager.Start()
}

// Check scheduled jobs
jobs := manager.scheduler.GetScheduledJobs()
fmt.Printf("Scheduled jobs: %v\n", jobs)
```

### Cost Data Not Deleted

**Symptoms**: Cost data remains after cleanup job runs

**Possible Causes**:
1. Deployment status not `destroyed` or `failed`
2. Database permissions issue
3. Collection names incorrect

**Resolution**:
```go
// Check deployment status
deployment, _ := app.Dao().FindRecordById("deployments", deploymentId)
fmt.Printf("Status: %s\n", deployment.GetString("status"))

// Manually trigger cleanup for specific deployment
scheduler := jobs.NewPricingScheduler(app)
deleted, err := scheduler.cleanupDeploymentCostData(deploymentId)
fmt.Printf("Deleted %d records, error: %v\n", deleted, err)
```

### Performance Issues

**Symptoms**: Cleanup job takes too long or times out

**Possible Causes**:
1. Too many destroyed deployments
2. Database performance issues
3. Missing indexes

**Resolution**:
1. Increase job timeout in configuration
2. Add database indexes on `deployment` and `status` fields
3. Consider batching cleanup operations
4. Run cleanup more frequently to reduce backlog

## Best Practices

1. **Monitor regularly**: Check job status and error logs daily
2. **Test before production**: Verify cleanup logic in staging environment
3. **Backup before cleanup**: Consider backing up cost data before deletion
4. **Audit trail**: Log all cleanup operations for compliance
5. **Gradual rollout**: Start with short retention periods, extend as needed

## Related Documentation

- [AWS Cost Estimation API](./AWS_COST_ESTIMATION_API.md)
- [Background Jobs System](./BACKGROUND_JOBS.md)
- [Database Schema](./DATABASE_SCHEMA.md)
- [Monitoring Dashboard](./AWS_COST_MONITORING_DASHBOARD.md)

## Support

For issues or questions about the cost cleanup job:

1. Check the logs: `pocketbase/logs/`
2. Review job status: Use the monitoring API
3. Run diagnostics: `go test -v ./pkg/jobs/...`
4. Contact support: Include job logs and error messages
