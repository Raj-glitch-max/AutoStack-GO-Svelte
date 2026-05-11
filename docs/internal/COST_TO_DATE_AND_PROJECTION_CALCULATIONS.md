# Cost-to-Date and Projected Monthly Cost Calculations

## Overview

This document explains how the AWS Cost Estimation system calculates cost-to-date and projected monthly costs for active deployments. These calculations are critical for providing users with accurate, real-time cost tracking and budget projections.

## Cost-to-Date Calculation

### Definition
**Cost-to-date** is the total actual AWS spending for a deployment from its creation date up to the most recent data available from AWS Cost Explorer (minus the 48-hour delay).

### Formula
```
costToDate = Σ(service costs across all days in period)
```

### Implementation
The cost-to-date is calculated by:
1. Fetching cost data from AWS Cost Explorer API
2. Aggregating costs across all services (EC2, S3, RDS, etc.)
3. Summing costs across all days in the period
4. Rounding to 2 decimal places for display

### Example
For a deployment that has been running for 10 days:
```
Day 1-10 costs:
  AmazonEC2: $5.00/day × 10 days = $50.00
  AmazonS3:  $0.50/day × 10 days = $5.00
  AmazonRDS: $3.00/day × 10 days = $30.00
  
Total cost-to-date = $50.00 + $5.00 + $30.00 = $85.00
```

### Service Breakdown
The cost-to-date includes a breakdown by AWS service:
```json
{
  "costToDate": 85.00,
  "breakdown": {
    "AmazonEC2": 50.00,
    "AmazonS3": 5.00,
    "AmazonRDS": 30.00
  }
}
```

## Projected Monthly Cost Calculation

### Definition
**Projected monthly cost** is an extrapolation of the cost-to-date to estimate what the total monthly cost will be, assuming the current spending rate continues.

### Formula
```
projectedMonthly = (costToDate / daysElapsed) × 30
```

Where:
- `costToDate` = Total spending so far
- `daysElapsed` = Number of days from deployment start to now (minus 48-hour delay)
- `30` = Standard month length for normalization

### Why 30 Days?
We use 30 days as a standard month length to normalize projections across different calendar months (28, 29, 30, or 31 days). This provides consistent, comparable monthly estimates.

### Implementation Details

#### Days Elapsed Calculation
```go
daysElapsed = (periodEnd - periodStart).Hours() / 24
```

#### Daily Average Calculation
```go
dailyAverage = costToDate / daysElapsed
```

#### Monthly Projection
```go
projectedMonthly = dailyAverage × 30
```

### Examples

#### Example 1: 10 Days Elapsed
```
Cost-to-date: $85.00
Days elapsed: 10
Daily average: $85.00 / 10 = $8.50/day
Projected monthly: $8.50 × 30 = $255.00
```

#### Example 2: First Day of Month
```
Cost-to-date: $8.50
Days elapsed: 1
Daily average: $8.50 / 1 = $8.50/day
Projected monthly: $8.50 × 30 = $255.00
```

#### Example 3: Mid-Month (15 Days)
```
Cost-to-date: $127.50
Days elapsed: 15
Daily average: $127.50 / 15 = $8.50/day
Projected monthly: $8.50 × 30 = $255.00
```

#### Example 4: Full Month (30 Days)
```
Cost-to-date: $255.00
Days elapsed: 30
Daily average: $255.00 / 30 = $8.50/day
Projected monthly: $8.50 × 30 = $255.00
```

### Edge Cases Handled

#### 1. First Day of Month
When only 1 day of data is available, the projection is based on that single day's spending:
```
Cost-to-date: $5.00
Days elapsed: 1
Projected monthly: $5.00 / 1 × 30 = $150.00
```

**Note**: Early projections may be less accurate due to limited data. The system should display a confidence indicator for projections based on <7 days of data.

#### 2. Last Day of Month
When a full month of data is available, the projection equals the actual cost:
```
Cost-to-date: $255.00
Days elapsed: 30
Projected monthly: $255.00 / 30 × 30 = $255.00
```

#### 3. Zero Days Elapsed
If the period start and end are the same (edge case):
```go
if daysElapsed <= 0 {
    return costToDate  // Return as-is
}
```

#### 4. Zero Cost
If no costs have been incurred yet:
```
Cost-to-date: $0.00
Days elapsed: 5
Projected monthly: $0.00 / 5 × 30 = $0.00
```

#### 5. Very Small Time Periods (< 1 Day)
For periods less than 24 hours (e.g., 12 hours):
```
Cost-to-date: $2.50
Days elapsed: 0.5 (12 hours)
Daily average: $2.50 / 0.5 = $5.00/day
Projected monthly: $5.00 × 30 = $150.00
```

#### 6. Different Month Lengths
The system normalizes all months to 30 days:

**31-day month (January):**
```
Cost-to-date: $263.50 (31 days)
Days elapsed: 31
Daily average: $263.50 / 31 = $8.50/day
Projected monthly: $8.50 × 30 = $255.00
```

**28-day month (February):**
```
Cost-to-date: $238.00 (28 days)
Days elapsed: 28
Daily average: $238.00 / 28 = $8.50/day
Projected monthly: $8.50 × 30 = $255.00
```

## Variance Calculation

### Definition
**Variance** is the percentage difference between the projected monthly cost and the original estimate.

### Formula
```
variance = ((projectedMonthly - estimate) / estimate) × 100
```

### Examples

#### Example 1: On Budget
```
Projected monthly: $150.00
Estimate: $150.00
Variance: ((150 - 150) / 150) × 100 = 0%
```

#### Example 2: 20% Over Budget (Alert Threshold)
```
Projected monthly: $180.00
Estimate: $150.00
Variance: ((180 - 150) / 150) × 100 = +20%
```

#### Example 3: 10% Under Budget
```
Projected monthly: $135.00
Estimate: $150.00
Variance: ((135 - 150) / 150) × 100 = -10%
```

#### Example 4: Zero Estimate (Edge Case)
```
Projected monthly: $100.00
Estimate: $0.00
Variance: 0% (avoid division by zero)
```

### Variance Interpretation

| Variance Range | Status | Action |
|---------------|--------|--------|
| < -10% | Under budget 👍 | No action needed |
| -10% to +10% | On track ✅ | Monitor normally |
| +10% to +20% | Slightly over ⚠️ | Review spending |
| > +20% | Over budget 🚨 | Alert triggered |

## AWS Cost Explorer Delay

### 48-Hour Delay
AWS Cost Explorer data has a 48-hour delay. This means:
- Costs from today and yesterday are not yet available
- The most recent data is from 2 days ago
- Projections are based on data up to 48 hours ago

### Impact on Calculations
```
Today: January 15, 2024
Most recent data: January 13, 2024 (48 hours ago)

Period for calculation:
  Start: Deployment creation date
  End: January 13, 2024 (not January 15)
```

### User Communication
The UI should clearly indicate:
- "Cost data updated as of [date]"
- "AWS Cost Explorer has a 48-hour delay"
- "Most recent costs will appear in 2 days"

## Incremental Updates

### Strategy
Instead of fetching all cost data from deployment start every time, the system uses incremental updates:

1. **First Fetch**: Get all costs from deployment start to 48 hours ago
2. **Subsequent Fetches**: Get only new costs since last fetch
3. **Merge**: Combine new costs with existing costs

### Benefits
- Reduced API calls to AWS Cost Explorer
- Faster fetch times
- Lower AWS API costs
- Better rate limit compliance

### Example
```
Initial fetch (Day 5):
  Period: Day 1 to Day 3 (48h delay)
  Cost-to-date: $25.50

Incremental fetch (Day 10):
  Period: Day 3 to Day 8 (48h delay)
  New costs: $42.50
  Updated cost-to-date: $25.50 + $42.50 = $68.00
```

## Accuracy Considerations

### Early Projections (Days 1-7)
- **Accuracy**: Lower (±30-50%)
- **Reason**: Limited data, potential startup costs
- **Recommendation**: Display confidence indicator

### Mid-Month Projections (Days 8-20)
- **Accuracy**: Moderate (±15-25%)
- **Reason**: More data, patterns emerging
- **Recommendation**: Standard display

### Late-Month Projections (Days 21-30)
- **Accuracy**: High (±5-10%)
- **Reason**: Substantial data, clear patterns
- **Recommendation**: High confidence indicator

### Factors Affecting Accuracy
1. **Variable workloads**: Traffic spikes, batch jobs
2. **Scaling events**: Auto-scaling up/down
3. **Data transfer**: Unpredictable egress costs
4. **Reserved instances**: Upfront vs. ongoing costs
5. **Free tier**: First month may have lower costs

## Testing

### Unit Tests
The system includes comprehensive unit tests for:
- Projected monthly cost calculation
- Variance calculation
- Edge cases (zero days, zero cost, etc.)
- Boundary conditions
- Formula accuracy

### Test Coverage
- ✅ First day of month
- ✅ Last day of month
- ✅ Mid-month scenarios
- ✅ Zero cost handling
- ✅ Very small costs
- ✅ Very large costs
- ✅ Different month lengths (28, 29, 30, 31 days)
- ✅ Fractional days (partial days)
- ✅ Zero estimate handling
- ✅ Negative variance
- ✅ Large variance (>100%)

### Real-World Scenarios Tested
1. Typical web application (10 days in)
2. Static website (low cost)
3. Microservices (high cost)
4. First 48 hours (minimum Cost Explorer period)

## API Response Format

### Example Response
```json
{
  "actual": {
    "costToDate": 85.00,
    "projectedMonthly": 255.00,
    "variance": 20.00,
    "breakdown": {
      "AmazonEC2": 50.00,
      "AmazonS3": 5.00,
      "AmazonRDS": 30.00
    },
    "period": {
      "start": "2024-01-01T00:00:00Z",
      "end": "2024-01-13T00:00:00Z"
    },
    "fetchedAt": "2024-01-15T10:00:00Z"
  },
  "estimate": {
    "total": 212.50,
    "createdAt": "2024-01-01T10:00:00Z"
  }
}
```

## Best Practices

### For Users
1. **Review early projections with caution**: First week projections may be inaccurate
2. **Monitor variance trends**: Look for consistent patterns, not single-day spikes
3. **Set realistic estimates**: Include buffer for unexpected costs
4. **Check breakdown**: Identify which services are driving costs

### For Developers
1. **Always round to 2 decimals**: Consistent display format
2. **Handle zero estimates**: Avoid division by zero
3. **Validate period dates**: Ensure end > start
4. **Log calculation details**: Aid debugging and auditing
5. **Cache results**: Reduce API calls and improve performance

## Related Documentation
- [AWS Cost Estimation API](./api/API.md#actual-cost-endpoints)
- [Cost Explorer Error Handling](./COST_EXPLORER_ERROR_HANDLING.md)
- [Cost Data Freshness Monitoring](./COST_DATA_FRESHNESS_MONITORING.md)
- [AWS Cost Estimation User Guide](./AWS_COST_ESTIMATION_USER_GUIDE.md)

## References
- [AWS Cost Explorer API Documentation](https://docs.aws.amazon.com/cost-management/latest/APIReference/Welcome.html)
- [AWS Cost Explorer Data Delay](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-data-delay.html)
- Implementation: `pocketbase/pkg/aws/actual_cost_fetcher.go`
- Tests: `pocketbase/pkg/aws/actual_cost_projection_test.go`
