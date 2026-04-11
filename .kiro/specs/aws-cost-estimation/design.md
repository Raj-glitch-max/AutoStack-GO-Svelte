# AWS Cost Estimation - Design Document

## Architecture Overview

The AWS cost estimation system implements a three-tier architecture:

1. **Tier 1**: Pre-deployment estimates from cached AWS Pricing API data
2. **Tier 2**: Post-deployment actuals from AWS Cost Explorer API  
3. **Tier 3**: Anomaly detection and alerting via AWS Budgets integration

## System Components

### 1. Pricing Cache Service
- **Purpose**: Maintain fresh AWS pricing data per region
- **Data Source**: AWS Price List API
- **Update Frequency**: Every 24 hours via background job
- **Storage**: SQLite database with regional pricing tables

### 2. Cost Estimation Engine
- **Purpose**: Calculate pre-deployment cost ranges
- **Input**: Blueprint resources + region + usage assumptions
- **Output**: Cost range (min/max) with itemized breakdown
- **Performance**: <500ms response time from cache

### 3. Actual Cost Tracker
- **Purpose**: Fetch real AWS spending post-deployment
- **Data Source**: AWS Cost Explorer API
- **Update Frequency**: Daily after 48-hour AWS delay
- **Features**: Variance analysis, cost breakdown by service

### 4. Anomaly Detection System
- **Purpose**: Alert on cost overruns
- **Trigger**: Actual cost exceeds estimate by 20%
- **Actions**: Email notification + in-app alert
- **Configuration**: Per-deployment thresholds

## Data Model

### Database Schema

#### awsPricingCache Collection
```javascript
{
  id: "pricing_us_east_1_ec2_fargate",
  region: "us-east-1",
  service: "AmazonECS", 
  productFamily: "Compute",
  instanceType: "fargate",
  vcpu: 0.25,
  memory: 0.5,
  pricePerHour: 0.04048,
  pricePerMonth: 29.15,
  currency: "USD",
  fetchedAt: "2026-04-11T10:00:00Z",
  validUntil: "2026-04-12T10:00:00Z"
}
```
#### costEstimates Collection
```javascript
{
  id: "estimate_deploy_123",
  deployment: "deploy_123",
  blueprint: "static-website",
  region: "us-east-1",
  computeMonthly: 29.15,
  networkingMonthly: 16.20,
  storageMonthly: 2.30,
  transferMonthly: 9.00,
  totalEstimate: 56.65,
  rangeMin: 45.32,      // total * 0.8
  rangeMax: 79.31,      // total * 1.4
  assumptions: {
    "storage": "10GB",
    "transfer": "100GB/month",
    "requests": "10K/month"
  },
  disclaimer: "Excludes data transfer overages and CloudWatch detailed monitoring",
  createdAt: "2026-04-11T10:00:00Z",
  pricingVersion: "2026-04-11"
}
```

#### actualCosts Collection
```javascript
{
  id: "actual_deploy_123",
  deployment: "deploy_123", 
  costToDate: 47.23,
  projectedMonthly: 62.18,
  variance: 9.76,           // (actual - estimate) / estimate * 100
  breakdown: {
    "AmazonS3": 2.45,
    "AmazonCloudFront": 8.90,
    "AmazonRoute53": 0.50,
    "DataTransfer": 12.33
  },
  period: {
    start: "2026-04-01",
    end: "2026-04-11"
  },
  fetchedAt: "2026-04-11T10:00:00Z"
}
```

#### costAlerts Collection
```javascript
{
  id: "alert_deploy_123_001",
  deployment: "deploy_123",
  user: "user_456",
  type: "cost_overrun",
  threshold: 20.0,          // percentage
  triggered: true,
  actualCost: 62.18,
  estimatedCost: 56.65,
  variance: 9.76,
  message: "Deployment costs 9.8% above estimate due to higher data transfer",
  sentAt: "2026-04-11T10:00:00Z",
  acknowledged: false
}
```

## API Design

### Cost Estimation Endpoints

#### GET /api/cost/estimate
**Purpose**: Get pre-deployment cost estimate
**Parameters**:
- `blueprint`: Blueprint ID (required)
- `region`: AWS region (required) 
- `variables`: Terraform variables (optional)

**Response**:
```javascript
{
  "estimate": {
    "total": 56.65,
    "range": {
      "min": 45.32,
      "max": 79.31
    },
    "breakdown": {
      "compute": 29.15,
      "networking": 16.20,
      "storage": 2.30,
      "transfer": 9.00
    },
    "assumptions": {
      "storage": "10GB",
      "transfer": "100GB/month"
    },
    "disclaimer": "Excludes data transfer overages",
    "pricingFetchedAt": "2026-04-11T10:00:00Z"
  }
}
```

#### GET /api/cost/actual/{deploymentId}
**Purpose**: Get post-deployment actual costs
**Response**:
```javascript
{
  "actual": {
    "costToDate": 47.23,
    "projectedMonthly": 62.18,
    "variance": 9.76,
    "breakdown": {
      "AmazonS3": 2.45,
      "AmazonCloudFront": 8.90
    },
    "period": {
      "start": "2026-04-01", 
      "end": "2026-04-11"
    }
  },
  "estimate": {
    "total": 56.65,
    "createdAt": "2026-04-01T10:00:00Z"
  }
}
```
## Implementation Details

### 1. Pricing Cache Service

#### Background Job: AWS Pricing Fetcher
```go
type PricingFetcher struct {
    db       *sql.DB
    awsClient *pricing.Client
    regions  []string
}

func (pf *PricingFetcher) FetchPricing() error {
    for _, region := range pf.regions {
        // Fetch EC2/Fargate pricing
        err := pf.fetchFargateRates(region)
        if err != nil {
            log.Printf("Failed to fetch Fargate rates for %s: %v", region, err)
            continue
        }
        
        // Fetch S3 pricing
        err = pf.fetchS3Rates(region)
        if err != nil {
            log.Printf("Failed to fetch S3 rates for %s: %v", region, err)
            continue
        }
        
        // Fetch ALB pricing
        err = pf.fetchALBRates(region)
        if err != nil {
            log.Printf("Failed to fetch ALB rates for %s: %v", region, err)
            continue
        }
    }
    return nil
}
```

#### Cron Schedule
- **Frequency**: Daily at 2 AM UTC
- **Retry Logic**: Exponential backoff (1min, 2min, 4min, 8min)
- **Fallback**: Continue with stale data if all retries fail
- **Monitoring**: Alert if pricing data >48 hours old

### 2. Cost Calculation Engine

#### Service Cost Calculators
```go
type CostCalculator interface {
    Calculate(blueprint Blueprint, region string, variables map[string]interface{}) (CostBreakdown, error)
}

type FargateCostCalculator struct {
    pricingCache *PricingCache
}

func (fcc *FargateCostCalculator) Calculate(vcpu, memory float64, region string) (float64, error) {
    rate, err := fcc.pricingCache.GetFargateRate(region, vcpu, memory)
    if err != nil {
        return 0, err
    }
    
    // Calculate monthly cost (24 hours * 30 days)
    monthlyHours := 24 * 30
    monthlyCost := rate.PricePerHour * float64(monthlyHours)
    
    return monthlyCost, nil
}
```

#### Blueprint Cost Mapping
```go
var BlueprintCostMapping = map[string][]ServiceCalculator{
    "static-website": {
        S3StorageCalculator{assumptions: map[string]interface{}{"storage_gb": 10}},
        S3RequestCalculator{assumptions: map[string]interface{}{"requests_per_month": 10000}},
        CloudFrontCalculator{assumptions: map[string]interface{}{"transfer_gb": 100}},
        Route53Calculator{assumptions: map[string]interface{}{"hosted_zones": 1}},
    },
    "web-application": {
        FargateCalculator{assumptions: map[string]interface{}{"vcpu": 0.25, "memory": 0.5}},
        ALBCalculator{assumptions: map[string]interface{}{"lcu_hours": 730}},
        RDSCalculator{assumptions: map[string]interface{}{"instance_class": "db.t3.micro"}},
        NATGatewayCalculator{assumptions: map[string]interface{}{"data_processed_gb": 50}},
        CloudWatchCalculator{assumptions: map[string]interface{}{"log_gb": 5}},
    },
}
```

### 3. AWS Cost Explorer Integration

#### Actual Cost Fetcher
```go
type ActualCostFetcher struct {
    costExplorerClient *costexplorer.Client
    db                 *sql.DB
}

func (acf *ActualCostFetcher) FetchActualCosts(deploymentID string) error {
    deployment, err := acf.getDeployment(deploymentID)
    if err != nil {
        return err
    }
    
    // Cost Explorer requires 48-hour delay
    if time.Since(deployment.CreatedAt) < 48*time.Hour {
        return errors.New("cost data not yet available")
    }
    
    // Fetch costs with deployment tags
    input := &costexplorer.GetCostAndUsageInput{
        TimePeriod: &types.DateInterval{
            Start: aws.String(deployment.CreatedAt.Format("2006-01-02")),
            End:   aws.String(time.Now().Format("2006-01-02")),
        },
        Granularity: types.GranularityDaily,
        Metrics:     []string{"BlendedCost"},
        GroupBy: []types.GroupDefinition{
            {
                Type: types.GroupDefinitionTypeTag,
                Key:  aws.String("autostack:deployment"),
            },
            {
                Type: types.GroupDefinitionTypeDimension,
                Key:  aws.String("SERVICE"),
            },
        },
        Filter: &types.Expression{
            Tags: &types.TagValues{
                Key:    aws.String("autostack:deployment"),
                Values: []string{deploymentID},
            },
        },
    }
    
    result, err := acf.costExplorerClient.GetCostAndUsage(context.TODO(), input)
    if err != nil {
        return err
    }
    
    return acf.processResults(deploymentID, result)
}
```
### 4. Anomaly Detection System

#### Cost Monitor
```go
type CostMonitor struct {
    db            *sql.DB
    notifier      *notifications.Service
    alertThreshold float64 // default 20%
}

func (cm *CostMonitor) CheckCostAnomalies() error {
    deployments, err := cm.getActiveDeployments()
    if err != nil {
        return err
    }
    
    for _, deployment := range deployments {
        actual, err := cm.getActualCost(deployment.ID)
        if err != nil {
            continue // Skip if no actual cost data yet
        }
        
        estimate, err := cm.getEstimate(deployment.ID)
        if err != nil {
            continue
        }
        
        variance := (actual.ProjectedMonthly - estimate.Total) / estimate.Total * 100
        
        if variance > cm.alertThreshold {
            err := cm.sendCostAlert(deployment, actual, estimate, variance)
            if err != nil {
                log.Printf("Failed to send cost alert for deployment %s: %v", deployment.ID, err)
            }
        }
    }
    
    return nil
}

func (cm *CostMonitor) sendCostAlert(deployment Deployment, actual ActualCost, estimate CostEstimate, variance float64) error {
    alert := CostAlert{
        DeploymentID: deployment.ID,
        UserID:      deployment.UserID,
        Type:        "cost_overrun",
        Threshold:   cm.alertThreshold,
        ActualCost:  actual.ProjectedMonthly,
        EstimatedCost: estimate.Total,
        Variance:    variance,
        Message:     fmt.Sprintf("Deployment costs %.1f%% above estimate", variance),
    }
    
    // Save alert to database
    err := cm.saveAlert(alert)
    if err != nil {
        return err
    }
    
    // Send email notification
    return cm.notifier.SendCostAlert(deployment.UserID, alert)
}
```

## Frontend Components

### Cost Estimator Component
```svelte
<!-- CostEstimator.svelte -->
<script>
  import { onMount } from 'svelte';
  
  export let blueprint;
  export let region;
  export let variables = {};
  
  let estimate = null;
  let loading = false;
  let error = null;
  
  async function fetchEstimate() {
    loading = true;
    error = null;
    
    try {
      const response = await fetch(`/api/cost/estimate?blueprint=${blueprint}&region=${region}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(variables)
      });
      
      if (!response.ok) throw new Error('Failed to fetch estimate');
      
      const data = await response.json();
      estimate = data.estimate;
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }
  
  onMount(fetchEstimate);
  
  $: if (blueprint || region) fetchEstimate();
</script>

<div class="cost-estimator">
  <h3>Estimated Monthly Cost</h3>
  
  {#if loading}
    <div class="loading">Calculating costs...</div>
  {:else if error}
    <div class="error">Error: {error}</div>
  {:else if estimate}
    <div class="estimate">
      <div class="total-range">
        <span class="range">${estimate.range.min} - ${estimate.range.max}</span>
        <span class="best-estimate">Best estimate: ${estimate.total}</span>
      </div>
      
      <div class="breakdown">
        <h4>Cost Breakdown</h4>
        <ul>
          <li>Compute: ${estimate.breakdown.compute}</li>
          <li>Networking: ${estimate.breakdown.networking}</li>
          <li>Storage: ${estimate.breakdown.storage}</li>
          <li>Data Transfer: ${estimate.breakdown.transfer}</li>
        </ul>
      </div>
      
      <div class="assumptions">
        <h4>Assumptions</h4>
        <ul>
          {#each Object.entries(estimate.assumptions) as [key, value]}
            <li>{key}: {value}</li>
          {/each}
        </ul>
      </div>
      
      <div class="disclaimer">
        <p><strong>Note:</strong> {estimate.disclaimer}</p>
        <p class="pricing-age">
          Pricing data from {new Date(estimate.pricingFetchedAt).toLocaleDateString()}
        </p>
      </div>
    </div>
  {/if}
</div>

<style>
  .cost-estimator {
    border: 1px solid #e0e0e0;
    border-radius: 8px;
    padding: 16px;
    margin: 16px 0;
  }
  
  .total-range {
    font-size: 1.2em;
    margin-bottom: 16px;
  }
  
  .range {
    font-weight: bold;
    color: #2196F3;
  }
  
  .best-estimate {
    display: block;
    font-size: 0.9em;
    color: #666;
    margin-top: 4px;
  }
  
  .breakdown, .assumptions {
    margin: 16px 0;
  }
  
  .breakdown ul, .assumptions ul {
    list-style: none;
    padding: 0;
  }
  
  .breakdown li, .assumptions li {
    padding: 4px 0;
    border-bottom: 1px solid #f0f0f0;
  }
  
  .disclaimer {
    background: #fff3cd;
    border: 1px solid #ffeaa7;
    border-radius: 4px;
    padding: 12px;
    margin-top: 16px;
  }
  
  .disclaimer p {
    margin: 4px 0;
    font-size: 0.9em;
  }
  
  .pricing-age {
    color: #666;
    font-style: italic;
  }
  
  .loading, .error {
    text-align: center;
    padding: 20px;
  }
  
  .error {
    color: #d32f2f;
    background: #ffebee;
    border-radius: 4px;
  }
</style>
```
### Actual Cost Display Component
```svelte
<!-- ActualCostDisplay.svelte -->
<script>
  import { onMount } from 'svelte';
  
  export let deploymentId;
  
  let actualCost = null;
  let loading = false;
  let error = null;
  
  async function fetchActualCost() {
    loading = true;
    error = null;
    
    try {
      const response = await fetch(`/api/cost/actual/${deploymentId}`);
      
      if (!response.ok) {
        if (response.status === 404) {
          error = "Cost data not yet available (AWS requires 48-hour delay)";
          return;
        }
        throw new Error('Failed to fetch actual costs');
      }
      
      const data = await response.json();
      actualCost = data;
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }
  
  onMount(fetchActualCost);
  
  function getVarianceColor(variance) {
    if (variance < -10) return '#4caf50'; // Green for under budget
    if (variance < 10) return '#ff9800';  // Orange for close to estimate
    return '#f44336'; // Red for over budget
  }
</script>

<div class="actual-cost-display">
  <h3>Actual Costs</h3>
  
  {#if loading}
    <div class="loading">Loading actual costs...</div>
  {:else if error}
    <div class="info">{error}</div>
  {:else if actualCost}
    <div class="cost-comparison">
      <div class="actual">
        <h4>Current Spend</h4>
        <div class="amount">${actualCost.actual.costToDate}</div>
        <div class="projected">Projected monthly: ${actualCost.actual.projectedMonthly}</div>
      </div>
      
      <div class="estimate">
        <h4>Original Estimate</h4>
        <div class="amount">${actualCost.estimate.total}</div>
        <div class="created">From {new Date(actualCost.estimate.createdAt).toLocaleDateString()}</div>
      </div>
      
      <div class="variance" style="color: {getVarianceColor(actualCost.actual.variance)}">
        <h4>Variance</h4>
        <div class="percentage">
          {actualCost.actual.variance > 0 ? '+' : ''}{actualCost.actual.variance.toFixed(1)}%
        </div>
        <div class="status">
          {#if actualCost.actual.variance < -10}
            Under budget 👍
          {:else if actualCost.actual.variance < 10}
            On track ✅
          {:else}
            Over budget ⚠️
          {/if}
        </div>
      </div>
    </div>
    
    <div class="breakdown">
      <h4>Cost by Service</h4>
      <div class="service-costs">
        {#each Object.entries(actualCost.actual.breakdown) as [service, cost]}
          <div class="service-item">
            <span class="service-name">{service}</span>
            <span class="service-cost">${cost}</span>
          </div>
        {/each}
      </div>
    </div>
    
    <div class="period">
      <p>Period: {actualCost.actual.period.start} to {actualCost.actual.period.end}</p>
    </div>
  {/if}
</div>

<style>
  .actual-cost-display {
    border: 1px solid #e0e0e0;
    border-radius: 8px;
    padding: 16px;
    margin: 16px 0;
  }
  
  .cost-comparison {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 16px;
    margin-bottom: 24px;
  }
  
  .actual, .estimate, .variance {
    text-align: center;
    padding: 16px;
    border-radius: 8px;
    background: #f9f9f9;
  }
  
  .amount {
    font-size: 1.5em;
    font-weight: bold;
    margin: 8px 0;
  }
  
  .projected, .created, .status {
    font-size: 0.9em;
    color: #666;
  }
  
  .percentage {
    font-size: 1.3em;
    font-weight: bold;
    margin: 8px 0;
  }
  
  .service-costs {
    display: grid;
    gap: 8px;
  }
  
  .service-item {
    display: flex;
    justify-content: space-between;
    padding: 8px;
    background: #f5f5f5;
    border-radius: 4px;
  }
  
  .service-name {
    font-weight: 500;
  }
  
  .service-cost {
    font-weight: bold;
  }
  
  .period {
    margin-top: 16px;
    padding-top: 16px;
    border-top: 1px solid #e0e0e0;
    font-size: 0.9em;
    color: #666;
  }
  
  .loading, .info {
    text-align: center;
    padding: 20px;
    color: #666;
    background: #f9f9f9;
    border-radius: 4px;
  }
</style>
```

## Error Handling & Edge Cases

### Pricing API Failures
1. **Retry Logic**: Exponential backoff with jitter
2. **Fallback**: Use stale pricing data with warning
3. **Monitoring**: Alert if pricing data >48 hours old
4. **User Communication**: Show pricing data age in UI

### Cost Explorer Delays
1. **48-Hour Rule**: Don't fetch costs until 48 hours after deployment
2. **User Messaging**: Clear explanation of AWS delay
3. **Progressive Enhancement**: Show estimate until actuals available
4. **Retry Logic**: Daily attempts to fetch actual costs

### Regional Pricing Variations
1. **Per-Region Cache**: Store pricing data separately per region
2. **Validation**: Ensure selected region has pricing data
3. **Fallback Regions**: Use us-east-1 pricing if region unavailable
4. **User Warning**: Indicate when using fallback pricing

### Cost Calculation Edge Cases
1. **Missing Pricing**: Graceful degradation with partial estimates
2. **Zero Costs**: Handle free tier and zero-cost resources
3. **Negative Costs**: Handle credits and refunds in actuals
4. **Currency Conversion**: All calculations in USD

## Performance Considerations

### Caching Strategy
- **Pricing Cache**: 24-hour TTL, background refresh
- **Estimate Cache**: 1-hour TTL per blueprint/region combination
- **Actual Cost Cache**: 6-hour TTL (Cost Explorer rate limits)

### Database Optimization
- **Indexes**: On region, service, fetchedAt columns
- **Partitioning**: Separate tables per region for large datasets
- **Cleanup**: Archive pricing data older than 90 days

### API Rate Limits
- **AWS Pricing API**: 10 requests/second, implement backoff
- **Cost Explorer API**: 5 requests/second, queue requests
- **Circuit Breaker**: Fail fast if APIs consistently unavailable

## Security Considerations

### Data Protection
- **No Billing Data Storage**: Never store actual AWS billing information
- **User Isolation**: Cost data scoped to deployment owner only
- **Encryption**: Sensitive cost data encrypted at rest
- **Audit Trail**: Log all cost data access

### API Security
- **Authentication**: All cost endpoints require valid user session
- **Authorization**: Users can only access their own deployment costs
- **Rate Limiting**: Prevent abuse of cost calculation endpoints
- **Input Validation**: Sanitize all blueprint and region parameters

## Monitoring & Alerting

### System Health Metrics
- **Pricing Fetch Success Rate**: >95% success rate
- **Estimate Accuracy**: Variance within 30% for 90% of deployments
- **API Response Times**: <500ms for estimates, <2s for actuals
- **Data Freshness**: Pricing data never >48 hours old

### Business Metrics
- **Cost Estimate Usage**: % of deployments that view estimates
- **Actual Cost Adoption**: % of users who check actual costs
- **Alert Effectiveness**: % of cost overruns caught by alerts
- **User Trust**: Survey feedback on estimate accuracy

## Testing Strategy

### Unit Tests
- Cost calculation logic for each service type
- Pricing cache refresh mechanisms
- Variance calculation accuracy
- Error handling for API failures

### Integration Tests
- End-to-end estimate generation
- AWS API integration (with mocks)
- Database operations and caching
- Alert generation and delivery

### Load Tests
- Concurrent estimate requests
- Pricing cache performance under load
- Database query performance
- API rate limit handling

This design provides enterprise-grade cost estimation that builds user trust through accuracy, transparency, and proactive monitoring.