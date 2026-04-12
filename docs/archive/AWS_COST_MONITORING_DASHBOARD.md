# AWS Cost Estimation - Monitoring Dashboard Configuration

## Overview

This document defines the monitoring dashboard configuration for the AWS Cost Estimation system. It includes metrics, alerts, and visualization recommendations for operational visibility.

## Dashboard Layout

### Section 1: System Health Overview

**Purpose**: Quick health check of the entire cost estimation system

**Metrics**:
1. Overall System Status (Green/Yellow/Red)
2. Pricing Cache Status
3. Cost Estimation API Availability
4. Alert Delivery Success Rate
5. Last Pricing Fetch Timestamp

**Visualization**: Status panel with color-coded indicators

---

### Section 2: Pricing Cache Metrics

#### 2.1 Pricing Data Freshness

**Metric**: `pricing_cache_age_hours`

**Query**:
```sql
SELECT 
  region,
  ROUND((JULIANDAY('now') - JULIANDAY(fetchedAt)) * 24, 2) as age_hours
FROM awsPricingCache
GROUP BY region
ORDER BY age_hours DESC
```

**Visualization**: Time series graph showing age per region

**Alert Thresholds**:
- Warning: > 36 hours
- Critical: > 48 hours

---

#### 2.2 Pricing Fetch Success Rate

**Metric**: `pricing_fetch_success_rate`

**Calculation**:
```
Success Rate = (Successful Fetches / Total Fetch Attempts) * 100
```

**Query**:
```sql
SELECT 
  DATE(timestamp) as date,
  SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*) as success_rate
FROM pricingFetchJobs
WHERE timestamp > datetime('now', '-7 days')
GROUP BY DATE(timestamp)
ORDER BY date DESC
```

**Visualization**: Line chart with 7-day trend

**Alert Thresholds**:
- Warning: < 95%
- Critical: < 90%

---

#### 2.3 Pricing Cache Size

**Metric**: `pricing_cache_size_mb`

**Query**:
```sql
SELECT 
  region,
  COUNT(*) as price_points,
  ROUND(SUM(LENGTH(pricePerUnit)) / 1024.0 / 1024.0, 2) as size_mb
FROM awsPricingCache
GROUP BY region
```

**Visualization**: Bar chart showing size per region

**Alert Thresholds**:
- Warning: > 10 MB per region
- Critical: > 20 MB per region

---

#### 2.4 Regional Coverage

**Metric**: `pricing_regions_covered`

**Query**:
```sql
SELECT 
  COUNT(DISTINCT region) as regions_covered,
  GROUP_CONCAT(DISTINCT region) as regions
FROM awsPricingCache
WHERE fetchedAt > datetime('now', '-48 hours')
```

**Visualization**: Single stat panel

**Expected Value**: 8 regions (us-east-1, us-east-2, us-west-1, us-west-2, eu-west-1, eu-central-1, ap-southeast-1, ap-northeast-1)

---

### Section 3: Cost Estimation API Metrics

#### 3.1 Request Rate

**Metric**: `cost_estimate_requests_per_minute`

**Query**:
```sql
SELECT 
  strftime('%Y-%m-%d %H:%M', timestamp) as minute,
  COUNT(*) as requests
FROM apiLogs
WHERE endpoint = '/api/cost/estimate'
  AND timestamp > datetime('now', '-1 hour')
GROUP BY minute
ORDER BY minute DESC
```

**Visualization**: Time series graph with 1-hour window

**Normal Range**: 10-100 requests/minute

---

#### 3.2 Response Time

**Metric**: `cost_estimate_response_time_ms`

**Query**:
```sql
SELECT 
  strftime('%Y-%m-%d %H:%M', timestamp) as minute,
  AVG(responseTime) as avg_ms,
  MAX(responseTime) as max_ms,
  MIN(responseTime) as min_ms,
  PERCENTILE(responseTime, 95) as p95_ms,
  PERCENTILE(responseTime, 99) as p99_ms
FROM apiLogs
WHERE endpoint = '/api/cost/estimate'
  AND timestamp > datetime('now', '-1 hour')
GROUP BY minute
ORDER BY minute DESC
```

**Visualization**: Multi-line time series (avg, p95, p99)

**Alert Thresholds**:
- Warning: P95 > 500ms
- Critical: P95 > 1000ms

---

#### 3.3 Error Rate

**Metric**: `cost_estimate_error_rate`

**Query**:
```sql
SELECT 
  strftime('%Y-%m-%d %H', timestamp) as hour,
  SUM(CASE WHEN statusCode >= 500 THEN 1 ELSE 0 END) * 100.0 / COUNT(*) as error_rate
FROM apiLogs
WHERE endpoint = '/api/cost/estimate'
  AND timestamp > datetime('now', '-24 hours')
GROUP BY hour
ORDER BY hour DESC
```

**Visualization**: Line chart with 24-hour trend

**Alert Thresholds**:
- Warning: > 1%
- Critical: > 5%

---

#### 3.4 Cache Hit Rate

**Metric**: `cost_estimate_cache_hit_rate`

**Query**:
```sql
SELECT 
  strftime('%Y-%m-%d %H', timestamp) as hour,
  SUM(CASE WHEN cacheHit = 1 THEN 1 ELSE 0 END) * 100.0 / COUNT(*) as hit_rate
FROM apiLogs
WHERE endpoint = '/api/cost/estimate'
  AND timestamp > datetime('now', '-24 hours')
GROUP BY hour
ORDER BY hour DESC
```

**Visualization**: Line chart with 24-hour trend

**Target**: > 80% cache hit rate

---

### Section 4: Cost Accuracy Metrics

#### 4.1 Average Variance

**Metric**: `cost_variance_percentage_avg`

**Query**:
```sql
SELECT 
  AVG(variancePercentage) as avg_variance,
  STDDEV(variancePercentage) as stddev_variance,
  MIN(variancePercentage) as min_variance,
  MAX(variancePercentage) as max_variance
FROM actualCosts
WHERE dataFetchedAt > datetime('now', '-30 days')
  AND estimatedCost > 0
```

**Visualization**: Single stat panel with trend

**Target**: < 20% average variance

---

#### 4.2 Variance Distribution

**Metric**: `cost_variance_distribution`

**Query**:
```sql
SELECT 
  CASE 
    WHEN ABS(variancePercentage) <= 10 THEN '0-10%'
    WHEN ABS(variancePercentage) <= 20 THEN '10-20%'
    WHEN ABS(variancePercentage) <= 30 THEN '20-30%'
    ELSE '>30%'
  END as variance_bucket,
  COUNT(*) as count,
  COUNT(*) * 100.0 / (SELECT COUNT(*) FROM actualCosts WHERE dataFetchedAt > datetime('now', '-30 days')) as percentage
FROM actualCosts
WHERE dataFetchedAt > datetime('now', '-30 days')
  AND estimatedCost > 0
GROUP BY variance_bucket
ORDER BY variance_bucket
```

**Visualization**: Pie chart or bar chart

**Target**: 
- 0-10%: > 70%
- 10-20%: > 20%
- 20-30%: < 8%
- >30%: < 2%

---

#### 4.3 Estimates Within Range

**Metric**: `estimates_within_range_percentage`

**Query**:
```sql
SELECT 
  SUM(CASE 
    WHEN actualCost >= rangeMin AND actualCost <= rangeMax THEN 1 
    ELSE 0 
  END) * 100.0 / COUNT(*) as within_range_percentage
FROM actualCosts
WHERE dataFetchedAt > datetime('now', '-30 days')
  AND estimatedCost > 0
```

**Visualization**: Single stat panel with gauge

**Target**: > 90%

---

#### 4.4 Service-Level Variance

**Metric**: `service_variance_breakdown`

**Query**:
```sql
SELECT 
  service,
  AVG(variancePercentage) as avg_variance,
  COUNT(*) as sample_count
FROM actualCosts
WHERE dataFetchedAt > datetime('now', '-30 days')
  AND estimatedCost > 0
GROUP BY service
ORDER BY avg_variance DESC
```

**Visualization**: Bar chart showing variance per service

**Use Case**: Identify which services have the most inaccurate estimates

---

### Section 5: Alert System Metrics

#### 5.1 Active Alerts

**Metric**: `active_cost_alerts_count`

**Query**:
```sql
SELECT 
  COUNT(*) as active_alerts,
  SUM(CASE WHEN severity = 'critical' THEN 1 ELSE 0 END) as critical_count,
  SUM(CASE WHEN severity = 'warning' THEN 1 ELSE 0 END) as warning_count
FROM costAlerts
WHERE status = 'active'
```

**Visualization**: Single stat panel with breakdown

**Alert Thresholds**:
- Warning: > 10 active alerts
- Critical: > 50 active alerts

---

#### 5.2 Alert Delivery Success Rate

**Metric**: `alert_delivery_success_rate`

**Query**:
```sql
SELECT 
  DATE(createdAt) as date,
  SUM(CASE WHEN deliveryStatus = 'delivered' THEN 1 ELSE 0 END) * 100.0 / COUNT(*) as success_rate
FROM costAlerts
WHERE createdAt > datetime('now', '-7 days')
GROUP BY DATE(createdAt)
ORDER BY date DESC
```

**Visualization**: Line chart with 7-day trend

**Alert Thresholds**:
- Warning: < 95%
- Critical: < 90%

---

#### 5.3 Alert Response Time

**Metric**: `alert_acknowledgment_time_hours`

**Query**:
```sql
SELECT 
  AVG(JULIANDAY(acknowledgedAt) - JULIANDAY(createdAt)) * 24 as avg_hours,
  MAX(JULIANDAY(acknowledgedAt) - JULIANDAY(createdAt)) * 24 as max_hours
FROM costAlerts
WHERE acknowledgedAt IS NOT NULL
  AND createdAt > datetime('now', '-7 days')
```

**Visualization**: Single stat panel

**Target**: < 4 hours average

---

#### 5.4 Alert Frequency by Deployment

**Metric**: `alerts_per_deployment`

**Query**:
```sql
SELECT 
  d.name as deployment_name,
  COUNT(a.id) as alert_count,
  MAX(a.createdAt) as last_alert
FROM costAlerts a
JOIN deployments d ON a.deploymentId = d.id
WHERE a.createdAt > datetime('now', '-30 days')
GROUP BY d.id, d.name
ORDER BY alert_count DESC
LIMIT 10
```

**Visualization**: Table showing top 10 deployments with most alerts

**Use Case**: Identify problematic deployments

---

### Section 6: Actual Cost Tracking Metrics

#### 6.1 Cost Explorer Fetch Success Rate

**Metric**: `cost_explorer_fetch_success_rate`

**Query**:
```sql
SELECT 
  DATE(timestamp) as date,
  SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*) as success_rate
FROM costExplorerFetchJobs
WHERE timestamp > datetime('now', '-7 days')
GROUP BY DATE(timestamp)
ORDER BY date DESC
```

**Visualization**: Line chart with 7-day trend

**Alert Thresholds**:
- Warning: < 95%
- Critical: < 90%

---

#### 6.2 Cost Data Freshness

**Metric**: `actual_cost_data_age_hours`

**Query**:
```sql
SELECT 
  deploymentId,
  ROUND((JULIANDAY('now') - JULIANDAY(dataFetchedAt)) * 24, 2) as age_hours
FROM actualCosts
WHERE dataFetchedAt IS NOT NULL
ORDER BY age_hours DESC
LIMIT 10
```

**Visualization**: Table showing oldest cost data

**Alert Thresholds**:
- Warning: > 30 hours
- Critical: > 48 hours

---

#### 6.3 Deployments Without Cost Data

**Metric**: `deployments_missing_cost_data`

**Query**:
```sql
SELECT 
  COUNT(*) as missing_count
FROM deployments d
LEFT JOIN actualCosts ac ON d.id = ac.deploymentId
WHERE d.status = 'active'
  AND d.createdAt < datetime('now', '-72 hours')
  AND ac.id IS NULL
```

**Visualization**: Single stat panel

**Alert Thresholds**:
- Warning: > 5 deployments
- Critical: > 20 deployments

---

### Section 7: Performance Metrics

#### 7.1 Database Query Performance

**Metric**: `database_query_time_ms`

**Query**:
```sql
SELECT 
  queryType,
  AVG(executionTime) as avg_ms,
  MAX(executionTime) as max_ms,
  COUNT(*) as query_count
FROM queryLogs
WHERE timestamp > datetime('now', '-1 hour')
GROUP BY queryType
ORDER BY avg_ms DESC
```

**Visualization**: Table showing slowest queries

**Alert Thresholds**:
- Warning: Average > 100ms
- Critical: Average > 500ms

---

#### 7.2 API Throughput

**Metric**: `api_requests_per_second`

**Query**:
```sql
SELECT 
  strftime('%Y-%m-%d %H:%M', timestamp) as minute,
  COUNT(*) / 60.0 as rps
FROM apiLogs
WHERE timestamp > datetime('now', '-1 hour')
GROUP BY minute
ORDER BY minute DESC
```

**Visualization**: Time series graph

**Normal Range**: 1-10 RPS

---

#### 7.3 Memory Usage

**Metric**: `application_memory_mb`

**Source**: System metrics (Prometheus, CloudWatch, etc.)

**Visualization**: Time series graph

**Alert Thresholds**:
- Warning: > 512 MB
- Critical: > 1024 MB

---

#### 7.4 CPU Usage

**Metric**: `application_cpu_percentage`

**Source**: System metrics (Prometheus, CloudWatch, etc.)

**Visualization**: Time series graph

**Alert Thresholds**:
- Warning: > 70%
- Critical: > 90%

---

## Alert Configuration

### Critical Alerts (Page On-Call)

1. **Pricing Data Stale > 48 Hours**
   - Condition: `pricing_cache_age_hours > 48`
   - Action: Page on-call engineer
   - Runbook: [Stale Pricing Data](#issue-1-stale-pricing-data)

2. **Cost Estimation API Error Rate > 5%**
   - Condition: `cost_estimate_error_rate > 5`
   - Action: Page on-call engineer
   - Runbook: [Estimate Endpoint Errors](#issue-4-estimate-endpoint-returning-500-error)

3. **Alert Delivery Failure > 10%**
   - Condition: `alert_delivery_success_rate < 90`
   - Action: Page on-call engineer
   - Runbook: [Alert System Problems](#alert-system-problems)

### Warning Alerts (Slack Notification)

1. **Pricing Data Stale > 36 Hours**
   - Condition: `pricing_cache_age_hours > 36`
   - Action: Notify #ops-alerts channel

2. **Cost Estimation API P95 > 500ms**
   - Condition: `cost_estimate_response_time_p95 > 500`
   - Action: Notify #ops-alerts channel

3. **Average Variance > 25%**
   - Condition: `cost_variance_percentage_avg > 25`
   - Action: Notify #cost-estimation channel

4. **Active Alerts > 10**
   - Condition: `active_cost_alerts_count > 10`
   - Action: Notify #ops-alerts channel

### Info Alerts (Dashboard Only)

1. **Cache Hit Rate < 80%**
   - Condition: `cost_estimate_cache_hit_rate < 80`
   - Action: Display on dashboard

2. **Deployments Missing Cost Data > 5**
   - Condition: `deployments_missing_cost_data > 5`
   - Action: Display on dashboard

---

## Dashboard Implementation

### Grafana Configuration

```yaml
dashboard:
  title: "AWS Cost Estimation System"
  refresh: "30s"
  time:
    from: "now-24h"
    to: "now"
  
  rows:
    - title: "System Health"
      panels:
        - type: "stat"
          title: "Overall Status"
          targets:
            - query: "SELECT status FROM system_health"
        
        - type: "stat"
          title: "Pricing Cache Age"
          targets:
            - query: "SELECT MAX(age_hours) FROM pricing_cache_age"
    
    - title: "Cost Estimation API"
      panels:
        - type: "graph"
          title: "Request Rate"
          targets:
            - query: "SELECT requests_per_minute FROM api_metrics"
        
        - type: "graph"
          title: "Response Time (P95)"
          targets:
            - query: "SELECT p95_ms FROM api_metrics"
    
    - title: "Cost Accuracy"
      panels:
        - type: "gauge"
          title: "Estimates Within Range"
          targets:
            - query: "SELECT within_range_percentage FROM accuracy_metrics"
        
        - type: "graph"
          title: "Average Variance"
          targets:
            - query: "SELECT avg_variance FROM accuracy_metrics"
```

### CloudWatch Dashboard

```json
{
  "widgets": [
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["AWS/Cost", "EstimateResponseTime", {"stat": "Average"}],
          ["...", {"stat": "p95"}]
        ],
        "period": 300,
        "stat": "Average",
        "region": "us-east-1",
        "title": "Cost Estimate Response Time"
      }
    },
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["AWS/Cost", "PricingCacheAge", {"stat": "Maximum"}]
        ],
        "period": 3600,
        "stat": "Maximum",
        "region": "us-east-1",
        "title": "Pricing Cache Age"
      }
    }
  ]
}
```

---

## Monitoring Best Practices

1. **Set Realistic Thresholds**: Base alerts on historical data, not arbitrary values
2. **Reduce Alert Fatigue**: Only page for critical issues that require immediate action
3. **Document Runbooks**: Every alert should link to a runbook
4. **Review Regularly**: Update thresholds and metrics quarterly
5. **Track SLOs**: Define and monitor Service Level Objectives
6. **Correlate Metrics**: Look for patterns across multiple metrics
7. **Automate Responses**: Use automation for common issues (e.g., cache refresh)

---

## Service Level Objectives (SLOs)

| Metric | Target | Measurement Window |
|--------|--------|-------------------|
| Cost Estimate API Availability | 99.9% | 30 days |
| Cost Estimate Response Time (P95) | < 500ms | 7 days |
| Pricing Data Freshness | < 36 hours | Always |
| Estimate Accuracy (Within Range) | > 90% | 30 days |
| Alert Delivery Success Rate | > 95% | 7 days |
| Cost Explorer Fetch Success | > 95% | 7 days |

---

**Last Updated**: April 11, 2026  
**Version**: 1.0.0  
**Maintained By**: Platform Operations Team
