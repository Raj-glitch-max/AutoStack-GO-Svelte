# Daily Actual Cost Fetch Job

## Overview

The daily actual cost fetch job automatically retrieves actual AWS costs from the Cost Explorer API for all active deployments. This job runs daily at 3:00 AM UTC, one hour after the pricing fetch job.

## Implementation

### Job Configuration

- **Name**: `daily-actual-cost-fetch`
- **Schedule**: `0 0 3 * * *` (Every day at 3:00 AM UTC)
- **Timeout**: 30 minutes
- **Max Retries**: 3
- **Description**: Fetch actual AWS costs from Cost Explorer for active deployments

### Key Features

1. **Active Deployment Filtering**: Only fetches costs for deployments with `status = 'active'`
2. **48-Hour Delay Handling**: Respects AWS Cost Explorer's 48-hour data availability delay
3. **Error Handling**: Gracefully handles errors and continues processing other deployments
4. **Incremental Updates**: Updates existing cost records or creates new ones
5. **Variance Calculation**: Automatically calculates variance between actual and estimated costs

### Job Execution Flow

```
1. Start job at 3:00 AM UTC
2. Query database for all active deployments
3. For each deployment:
   a. Check if deployment is old enough (>48 hours)
   b. Fetch actual costs from AWS Cost Explorer
   c. Calculate projected monthly cost
   d. Calculate variance vs estimate
   e. Save/update actualCosts record
4. Log success/error counts
5. Record job completion metrics
```

### Integration with Existing Systems

The job integrates with:

- **ActualCostFetcher**: Uses `FetchActualCostsForActiveDeployments()` method
- **PricingScheduler**: Scheduled alongside other pricing jobs
- **JobMonitor**: Records execution metrics and health status
- **Database**: Reads from `deployments` and writes to `actualCosts` collections

## Usage

### Automatic Execution

The job runs automatically when the PricingScheduler is started:

```go
manager := NewPricingJobManager(app)
err := manager.Start()
```

### Manual Trigger

You can manually trigger the job for testing or immediate updates:

```go
manager := NewPricingJobManager(app)
err := manager.Start()
defer manager.Stop()

// Manually trigger actual cost fetch
err = manager.TriggerActualCostFetch()
```

### Monitoring

Check job status and health:

```go
// Get all job statuses
statuses := manager.GetJobStatus()

// Check system health
isHealthy, issues := manager.IsHealthy()
```

## Requirements Validation

This implementation validates the following acceptance criteria:

- **AC-3.6**: Updates daily automatically ✓
  - Job runs daily at 3:00 AM UTC
  - Automatically processes all active deployments

- **AC-3.1**: System fetches actual costs from AWS Cost Explorer API ✓
  - Uses ActualCostFetcher service
  - Integrates with AWS Cost Explorer SDK

- **AC-3.2**: Actual costs shown after 48 hours ✓
  - Validates deployment age before fetching
  - Skips deployments younger than 48 hours

- **AC-3.3**: Shows cost-to-date and projected monthly cost ✓
  - Calculates both metrics
  - Stores in actualCosts collection

- **AC-3.4**: Compares actual vs estimated with variance percentage ✓
  - Calculates variance automatically
  - Stores variance in database

## Error Handling

The job handles several error scenarios:

1. **No Active Deployments**: Completes successfully with log message
2. **Deployment Too Young**: Skips deployment with log message
3. **AWS API Errors**: Logs error and continues with next deployment
4. **Database Errors**: Logs error and continues processing
5. **Missing Estimates**: Calculates variance as 0 if estimate not found

## Testing

Comprehensive test suite in `actual_cost_job_test.go`:

- **TestActualCostFetchJob_Execution**: Verifies job executes without panic
- **TestActualCostFetchJob_Scheduling**: Confirms job is scheduled correctly
- **TestActualCostFetchJob_ManualTrigger**: Tests manual job triggering
- **TestActualCostFetchJob_Schedule**: Validates cron schedule
- **TestActualCostFetchJob_Timeout**: Confirms timeout configuration
- **TestActualCostFetchJob_ErrorHandling**: Tests error scenarios
- **TestActualCostFetchJob_Integration**: End-to-end integration test

Run tests:

```bash
cd pocketbase
go test -v ./pkg/jobs -run TestActualCostFetchJob
```

## Performance Considerations

- **Timeout**: 30-minute timeout allows processing many deployments
- **Parallel Processing**: Currently sequential; could be parallelized for scale
- **Rate Limiting**: AWS Cost Explorer has rate limits (5 requests/second)
- **Database Queries**: Uses indexed queries on deployment status

## Future Enhancements

Potential improvements for future iterations:

1. **Parallel Processing**: Process multiple deployments concurrently
2. **Incremental Updates**: Only fetch new data since last fetch
3. **Cost Anomaly Detection**: Trigger alerts during job execution
4. **Cleanup Job**: Remove cost data for destroyed deployments
5. **Metrics Dashboard**: Real-time job execution metrics
6. **Retry Logic**: Exponential backoff for failed deployments

## Related Files

- `pocketbase/pkg/jobs/pricing_fetch_job.go` - Job implementation
- `pocketbase/pkg/jobs/pricing_scheduler.go` - Job scheduling
- `pocketbase/pkg/jobs/actual_cost_job_test.go` - Test suite
- `pocketbase/pkg/aws/actual_cost_fetcher.go` - Cost fetching logic
- `.kiro/specs/aws-cost-estimation/tasks.md` - Task specification

## Troubleshooting

### Job Not Running

Check if scheduler is started:
```go
if !manager.scheduler.IsRunning() {
    log.Println("Scheduler is not running")
}
```

### No Costs Being Fetched

1. Verify deployments are active: `status = 'active'`
2. Check deployment age: Must be >48 hours old
3. Verify AWS credentials are configured
4. Check Cost Explorer API permissions

### High Error Rate

1. Review job logs for specific errors
2. Check AWS API rate limits
3. Verify database connectivity
4. Ensure deployment tags are correct

## Support

For issues or questions:
1. Check job logs in application output
2. Review test suite for expected behavior
3. Consult AWS Cost Explorer API documentation
4. Review related implementation files
