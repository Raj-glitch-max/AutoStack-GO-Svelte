# AWS Cost Estimation - Operational Runbooks

## Overview

This document provides step-by-step procedures for diagnosing and resolving common operational issues with the AWS Cost Estimation system.

## Table of Contents

1. [Pricing Data Issues](#pricing-data-issues)
2. [Cost Estimate Errors](#cost-estimate-errors)
3. [Actual Cost Tracking Issues](#actual-cost-tracking-issues)
4. [Alert System Problems](#alert-system-problems)
5. [Performance Issues](#performance-issues)
6. [Monitoring and Health Checks](#monitoring-and-health-checks)

---

## Pricing Data Issues

### Issue 1: Stale Pricing Data

**Symptoms**:
- Warning message: "Pricing data is stale (last updated X days ago)"
- Estimates may be inaccurate
- Admin dashboard shows failed pricing fetch

**Diagnosis**:

1. Check pricing cache status:
```bash
curl -X GET https://your-instance.com/api/admin/pricing/status \
  -H "Authorization: Bearer ADMIN_TOKEN"
```

2. Review job logs:
```bash
# Check PocketBase logs
tail -f pocketbase/pb_data/logs.db

# Look for pricing fetch errors
grep "pricing_fetch_job" pocketbase/pb_data/logs.db
```

3. Check AWS credentials:
```bash
# Verify AWS credentials are configured
aws sts get-caller-identity

# Test Price List API access
aws pricing get-products --service-code AmazonEC2 --region us-east-1
```

**Resolution**:

**Option A: Manual Refresh**
```bash
curl -X POST https://your-instance.com/api/admin/pricing/refresh \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"force": true}'
```

**Option B: Restart Background Job**
```bash
# Restart PocketBase to reinitialize cron jobs
systemctl restart pocketbase

# Or if running in Docker
docker restart pocketbase
```

**Option C: Fix AWS Credentials**
```bash
# Update AWS credentials
export AWS_ACCESS_KEY_ID="your-key"
export AWS_SECRET_ACCESS_KEY="your-secret"
export AWS_REGION="us-east-1"

# Restart service
systemctl restart pocketbase
```

**Prevention**:
- Set up monitoring alerts for stale pricing data (>36 hours)
- Configure AWS credential rotation
- Enable CloudWatch alarms for Price List API errors

---

### Issue 2: Pricing Fetch Job Failing

**Symptoms**:
- Repeated pricing fetch failures
- Error logs showing AWS API errors
- Pricing data not updating

**Diagnosis**:

1. Check job execution logs:
```bash
# View recent job executions
curl -X GET https://your-instance.com/api/admin/jobs/pricing \
  -H "Authorization: Bearer ADMIN_TOKEN"
```

2. Identify error type:
```bash
# Common error patterns
grep -E "rate limit|throttle|403|401" pocketbase/pb_data/logs.db
```

3. Test AWS API directly:
```bash
# Test Price List API
aws pricing get-products \
  --service-code AmazonEC2 \
  --filters "Type=TERM_MATCH,Field=location,Value=US East (N. Virginia)" \
  --region us-east-1
```

**Resolution**:

**Error: Rate Limit Exceeded**
```bash
# Increase backoff delay in pricing_scheduler.go
# Default: 1s, 2s, 4s, 8s, 16s
# Increase to: 2s, 4s, 8s, 16s, 32s

# Restart service
systemctl restart pocketbase
```

**Error: Invalid Credentials**
```bash
# Verify IAM permissions
aws iam get-user

# Required permissions:
# - pricing:GetProducts
# - pricing:DescribeServices

# Update IAM policy if needed
aws iam attach-user-policy \
  --user-name pocketbase-user \
  --policy-arn arn:aws:iam::aws:policy/AWSPriceListServiceFullAccess
```

**Error: Network Timeout**
```bash
# Increase timeout in pricing_fetcher.go
# Default: 30s
# Increase to: 60s

# Check network connectivity
curl -I https://pricing.us-east-1.amazonaws.com

# Restart service
systemctl restart pocketbase
```

**Prevention**:
- Implement exponential backoff with jitter
- Set up retry logic with circuit breaker
- Monitor AWS API health status
- Configure fallback to cached data

---

### Issue 3: Missing Regional Pricing

**Symptoms**:
- Cost estimates fail for specific regions
- Error: "Pricing data not available for region X"
- Some regions work, others don't

**Diagnosis**:

1. Check which regions have pricing data:
```bash
curl -X GET https://your-instance.com/api/admin/pricing/status \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  | jq '.regions'
```

2. Verify region is supported:
```bash
# List supported regions
aws ec2 describe-regions --region us-east-1
```

3. Check for region-specific fetch errors:
```bash
grep "region.*eu-west-1" pocketbase/pb_data/logs.db
```

**Resolution**:

**Option A: Fetch Missing Region**
```bash
curl -X POST https://your-instance.com/api/admin/pricing/refresh \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"regions": ["eu-west-1"], "force": true}'
```

**Option B: Add Region to Configuration**
```go
// In pocketbase/pkg/jobs/pricing_scheduler.go
var supportedRegions = []string{
    "us-east-1",
    "us-east-2",
    "us-west-1",
    "us-west-2",
    "eu-west-1",      // Add missing region
    "eu-central-1",
    "ap-southeast-1",
    "ap-northeast-1",
}
```

**Prevention**:
- Maintain list of supported regions in configuration
- Validate region before accepting deployment requests
- Set up alerts for missing regional pricing

---

## Cost Estimate Errors

### Issue 4: Estimate Endpoint Returning 500 Error

**Symptoms**:
- POST /api/cost/estimate returns 500 Internal Server Error
- Frontend shows "Failed to load cost estimate"
- Error logs show panic or exception

**Diagnosis**:

1. Check error logs:
```bash
tail -f pocketbase/pb_data/logs.db | grep "cost/estimate"
```

2. Test endpoint directly:
```bash
curl -X POST https://your-instance.com/api/cost/estimate \
  -H "Authorization: Bearer TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "blueprintId": "bp_static_website",
    "region": "us-east-1"
  }' \
  -v
```

3. Check database connectivity:
```bash
# Verify database is accessible
sqlite3 pocketbase/pb_data/data.db "SELECT COUNT(*) FROM awsPricingCache;"
```

**Resolution**:

**Error: Missing Pricing Data**
```bash
# Verify pricing cache has data
sqlite3 pocketbase/pb_data/data.db \
  "SELECT region, service, COUNT(*) FROM awsPricingCache GROUP BY region, service;"

# If empty, trigger pricing fetch
curl -X POST https://your-instance.com/api/admin/pricing/refresh \
  -H "Authorization: Bearer ADMIN_TOKEN"
```

**Error: Invalid Blueprint**
```bash
# Verify blueprint exists
sqlite3 pocketbase/pb_data/data.db \
  "SELECT id, name FROM blueprints WHERE id = 'bp_static_website';"

# Check blueprint configuration
curl -X GET https://your-instance.com/api/blueprints/bp_static_website \
  -H "Authorization: Bearer TOKEN"
```

**Error: Calculator Panic**
```bash
# Check for nil pointer dereferences
grep "panic" pocketbase/pb_data/logs.db

# Review calculator code for defensive checks
# Add nil checks in pocketbase/pkg/cost/*_calculator.go
```

**Prevention**:
- Add input validation middleware
- Implement graceful error handling in calculators
- Add health check endpoint for cost estimation
- Set up error rate monitoring

---

### Issue 5: Inaccurate Cost Estimates

**Symptoms**:
- Estimates significantly different from actual costs
- User complaints about surprise bills
- Variance consistently >30%

**Diagnosis**:

1. Compare estimate to actual:
```bash
# Get estimate
curl -X POST https://your-instance.com/api/cost/estimate \
  -H "Authorization: Bearer TOKEN" \
  -d '{"blueprintId": "bp_web_app", "region": "us-east-1"}'

# Get actual (after deployment)
curl -X GET https://your-instance.com/api/cost/actual/dep_123 \
  -H "Authorization: Bearer TOKEN"
```

2. Analyze variance by service:
```bash
# Query actual costs breakdown
sqlite3 pocketbase/pb_data/data.db \
  "SELECT service, cost FROM actualCosts WHERE deploymentId = 'dep_123';"
```

3. Review assumptions:
```bash
# Check blueprint configuration
cat pocketbase/pkg/cost/blueprint_web_app.go
```

**Resolution**:

**Issue: Outdated Pricing**
```bash
# Force pricing refresh
curl -X POST https://your-instance.com/api/admin/pricing/refresh \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -d '{"force": true}'
```

**Issue: Incorrect Assumptions**
```go
// Update assumptions in blueprint file
// pocketbase/pkg/cost/blueprint_web_app.go

// Old assumption
dataTransferGB := 10

// New assumption (based on actual usage)
dataTransferGB := 25
```

**Issue: Missing Services**
```go
// Add missing service to blueprint
// pocketbase/pkg/cost/blueprint_web_app.go

// Add CloudWatch detailed monitoring
cloudwatchCost := calculateCloudWatchCost(config)
totalCost += cloudwatchCost
```

**Prevention**:
- Track estimate vs actual variance for all deployments
- Regularly review and update assumptions
- Add user feedback mechanism for inaccurate estimates
- Implement machine learning for usage prediction (future)

---

## Actual Cost Tracking Issues

### Issue 6: Actual Costs Not Appearing

**Symptoms**:
- Deployment >48 hours old but no actual cost data
- "Actual cost data will be available after..." message persists
- Cost Explorer integration not working

**Diagnosis**:

1. Check AWS Cost Explorer access:
```bash
# Test Cost Explorer API
aws ce get-cost-and-usage \
  --time-period Start=2026-04-01,End=2026-04-11 \
  --granularity DAILY \
  --metrics BlendedCost \
  --region us-east-1
```

2. Verify deployment tags:
```bash
# Check if deployment has proper tags
aws ecs describe-services \
  --cluster autostack-cluster \
  --services service-name \
  --region us-east-1 \
  | jq '.services[0].tags'
```

3. Check cost fetch job logs:
```bash
grep "actual_cost_fetch" pocketbase/pb_data/logs.db
```

**Resolution**:

**Error: Cost Explorer Not Enabled**
```bash
# Enable Cost Explorer in AWS Console
# Or via CLI (requires root account)
aws ce enable-cost-explorer
```

**Error: Missing IAM Permissions**
```bash
# Add required permissions
aws iam attach-user-policy \
  --user-name pocketbase-user \
  --policy-arn arn:aws:iam::aws:policy/AWSCostExplorerReadOnlyAccess
```

**Error: Missing Deployment Tags**
```bash
# Add tags to deployment
aws ecs tag-resource \
  --resource-arn arn:aws:ecs:us-east-1:123456789:service/cluster/service \
  --tags key=autostack:deployment,value=dep_123
```

**Prevention**:
- Validate Cost Explorer access during setup
- Implement automatic tagging for all deployments
- Set up monitoring for cost data freshness
- Add retry logic for Cost Explorer API failures

---

### Issue 7: Cost Data Delay >48 Hours

**Symptoms**:
- Deployment >72 hours old but still no cost data
- Cost Explorer returns empty results
- Users complaining about missing cost information

**Diagnosis**:

1. Check AWS Cost Explorer data availability:
```bash
# Query Cost Explorer directly
aws ce get-cost-and-usage \
  --time-period Start=2026-04-08,End=2026-04-11 \
  --granularity DAILY \
  --metrics BlendedCost \
  --filter file://filter.json \
  --region us-east-1

# filter.json
{
  "Tags": {
    "Key": "autostack:deployment",
    "Values": ["dep_123"]
  }
}
```

2. Verify deployment date:
```bash
sqlite3 pocketbase/pb_data/data.db \
  "SELECT id, createdAt FROM deployments WHERE id = 'dep_123';"
```

3. Check for Cost Explorer service issues:
```bash
# Check AWS Service Health Dashboard
curl https://status.aws.amazon.com/
```

**Resolution**:

**Issue: AWS Service Delay**
```
# Wait for AWS to process billing data
# Typical delay: 48-72 hours
# Maximum delay: 96 hours

# Show message to user
"Cost data is still being processed by AWS. Please check back in 24 hours."
```

**Issue: Incorrect Date Range**
```go
// Fix date range calculation in actual_cost_fetcher.go
// Ensure we're querying the correct time period

// Old code (incorrect)
startDate := deployment.CreatedAt

// New code (correct - account for 48h delay)
startDate := deployment.CreatedAt.Add(48 * time.Hour)
```

**Prevention**:
- Set user expectations about 48-72 hour delay
- Implement progressive disclosure (show estimate until actual available)
- Add monitoring for Cost Explorer API latency
- Cache Cost Explorer responses to reduce API calls

---

## Alert System Problems

### Issue 8: Cost Alerts Not Triggering

**Symptoms**:
- Actual costs exceed threshold but no alert sent
- Users not receiving email notifications
- Alert history shows no recent alerts

**Diagnosis**:

1. Check alert monitoring job:
```bash
# Verify job is running
grep "cost_monitor" pocketbase/pb_data/logs.db

# Check last execution time
curl -X GET https://your-instance.com/api/admin/jobs/cost-monitor \
  -H "Authorization: Bearer ADMIN_TOKEN"
```

2. Verify alert thresholds:
```bash
# Check user alert settings
sqlite3 pocketbase/pb_data/data.db \
  "SELECT userId, defaultThreshold FROM alertSettings;"
```

3. Test alert logic manually:
```bash
# Calculate variance
actual=142.75
estimate=125.50
variance=$(echo "scale=2; ($actual - $estimate) / $estimate * 100" | bc)
echo "Variance: $variance%"
```

**Resolution**:

**Issue: Job Not Running**
```bash
# Restart cron scheduler
systemctl restart pocketbase

# Verify job is scheduled
curl -X GET https://your-instance.com/api/admin/jobs \
  -H "Authorization: Bearer ADMIN_TOKEN"
```

**Issue: Email Service Down**
```bash
# Check email service configuration
grep "smtp" pocketbase/pb_data/logs.db

# Test email sending
curl -X POST https://your-instance.com/api/admin/test-email \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -d '{"to": "admin@example.com"}'
```

**Issue: Alert Deduplication**
```bash
# Check if alert already exists
sqlite3 pocketbase/pb_data/data.db \
  "SELECT * FROM costAlerts WHERE deploymentId = 'dep_123' AND status = 'active';"

# If duplicate, update existing alert instead of creating new
```

**Prevention**:
- Monitor alert job execution frequency
- Set up dead letter queue for failed notifications
- Implement alert delivery confirmation
- Add health check for email service

---

### Issue 9: Alert Spam

**Symptoms**:
- Users receiving multiple alerts for same issue
- Alert fatigue leading to ignored notifications
- Duplicate alerts in database

**Diagnosis**:

1. Check alert frequency:
```bash
# Count alerts per deployment
sqlite3 pocketbase/pb_data/data.db \
  "SELECT deploymentId, COUNT(*) FROM costAlerts 
   WHERE createdAt > datetime('now', '-7 days') 
   GROUP BY deploymentId 
   HAVING COUNT(*) > 5;"
```

2. Review deduplication logic:
```bash
# Check for duplicate alerts
sqlite3 pocketbase/pb_data/data.db \
  "SELECT deploymentId, type, COUNT(*) FROM costAlerts 
   WHERE status = 'active' 
   GROUP BY deploymentId, type 
   HAVING COUNT(*) > 1;"
```

**Resolution**:

**Issue: Missing Deduplication**
```go
// Add deduplication logic in cost_monitor.go

// Before creating alert, check for existing active alert
existingAlert := getActiveAlert(deploymentId, alertType)
if existingAlert != nil {
    // Update existing alert instead of creating new
    updateAlert(existingAlert.ID, newData)
    return
}

// Create new alert only if none exists
createAlert(deploymentId, alertType, data)
```

**Issue: No Cooldown Period**
```go
// Add cooldown period in cost_monitor.go

const alertCooldownHours = 24

// Check if alert was sent recently
lastAlert := getLastAlert(deploymentId, alertType)
if lastAlert != nil && time.Since(lastAlert.CreatedAt) < alertCooldownHours {
    // Skip sending alert
    return
}
```

**Prevention**:
- Implement alert deduplication by deployment + type
- Add configurable cooldown period (default: 24 hours)
- Allow users to snooze alerts
- Provide alert digest option (daily summary instead of real-time)

---

## Performance Issues

### Issue 10: Slow Cost Estimate Responses

**Symptoms**:
- Cost estimate endpoint taking >2 seconds
- Frontend showing loading spinner for extended time
- Users complaining about slow estimates

**Diagnosis**:

1. Measure response time:
```bash
# Test endpoint performance
time curl -X POST https://your-instance.com/api/cost/estimate \
  -H "Authorization: Bearer TOKEN" \
  -d '{"blueprintId": "bp_web_app", "region": "us-east-1"}'
```

2. Check database query performance:
```bash
# Enable query logging
sqlite3 pocketbase/pb_data/data.db "PRAGMA query_only = ON;"

# Analyze slow queries
sqlite3 pocketbase/pb_data/data.db "EXPLAIN QUERY PLAN 
  SELECT * FROM awsPricingCache WHERE region = 'us-east-1';"
```

3. Profile application:
```bash
# Enable Go profiling
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze profile
go tool pprof cpu.prof
```

**Resolution**:

**Issue: Missing Database Indexes**
```sql
-- Add indexes to awsPricingCache
CREATE INDEX IF NOT EXISTS idx_pricing_region_service 
  ON awsPricingCache(region, service);

CREATE INDEX IF NOT EXISTS idx_pricing_fetched_at 
  ON awsPricingCache(fetchedAt);
```

**Issue: No Response Caching**
```go
// Add caching in cost_estimate.go

var estimateCache = cache.New(1*time.Hour, 2*time.Hour)

func getCostEstimate(blueprintId, region string) (*CostEstimate, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("%s:%s", blueprintId, region)
    if cached, found := estimateCache.Get(cacheKey); found {
        return cached.(*CostEstimate), nil
    }
    
    // Calculate estimate
    estimate := calculateEstimate(blueprintId, region)
    
    // Cache result
    estimateCache.Set(cacheKey, estimate, cache.DefaultExpiration)
    
    return estimate, nil
}
```

**Issue: Inefficient Calculations**
```go
// Optimize calculator loops
// Before: O(n²) complexity
for _, service := range services {
    for _, price := range prices {
        if service.Type == price.Type {
            cost += calculateCost(service, price)
        }
    }
}

// After: O(n) complexity with map lookup
priceMap := make(map[string]Price)
for _, price := range prices {
    priceMap[price.Type] = price
}

for _, service := range services {
    if price, ok := priceMap[service.Type]; ok {
        cost += calculateCost(service, price)
    }
}
```

**Prevention**:
- Set performance budgets (<500ms for estimates)
- Monitor P95 and P99 response times
- Implement response caching with appropriate TTL
- Add database indexes for common queries
- Use connection pooling for database access

---

## Monitoring and Health Checks

### Health Check Endpoints

```bash
# Overall system health
curl https://your-instance.com/api/health

# Pricing cache health
curl https://your-instance.com/api/admin/pricing/status \
  -H "Authorization: Bearer ADMIN_TOKEN"

# Cost estimation health
curl https://your-instance.com/api/health/cost-estimation
```

### Key Metrics to Monitor

1. **Pricing Data Freshness**
```
Metric: time_since_last_pricing_fetch
Alert: > 36 hours
```

2. **Estimate Response Time**
```
Metric: cost_estimate_response_time_p95
Alert: > 500ms
```

3. **Estimate Accuracy**
```
Metric: cost_variance_percentage_avg
Alert: > 30%
```

4. **Alert Delivery Rate**
```
Metric: alert_delivery_success_rate
Alert: < 95%
```

5. **API Error Rate**
```
Metric: cost_api_error_rate
Alert: > 1%
```

### Monitoring Dashboard

Recommended metrics for dashboard:

```
- Pricing cache status (healthy/stale)
- Last pricing fetch timestamp
- Cost estimate request rate
- Cost estimate error rate
- Average estimate response time
- Active cost alerts count
- Alert delivery success rate
- Actual cost fetch success rate
- Average cost variance percentage
```

### Log Aggregation

Key log patterns to monitor:

```bash
# Pricing fetch failures
grep "pricing_fetch.*error" pocketbase/pb_data/logs.db

# Cost estimate errors
grep "cost/estimate.*500" pocketbase/pb_data/logs.db

# Alert delivery failures
grep "alert.*failed" pocketbase/pb_data/logs.db

# AWS API errors
grep "aws.*error" pocketbase/pb_data/logs.db
```

---

## Escalation Procedures

### Level 1: Automated Recovery

- Automatic retry with exponential backoff
- Fallback to cached data
- Circuit breaker activation

### Level 2: On-Call Engineer

- Manual pricing refresh
- Database query optimization
- Service restart

### Level 3: Platform Team

- Code fixes and deployment
- Infrastructure scaling
- AWS support ticket

### Level 4: AWS Support

- AWS API issues
- Cost Explorer problems
- Pricing API outages

---

**Last Updated**: April 11, 2026  
**Version**: 1.0.0  
**Maintained By**: Platform Operations Team
