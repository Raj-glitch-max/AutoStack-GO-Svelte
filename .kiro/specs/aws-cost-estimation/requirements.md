# AWS Cost Estimation - Requirements

## Overview

Implement production-grade AWS cost estimation that builds user trust through accuracy, transparency, and real-time tracking. The system must provide pre-deployment estimates, post-deployment actuals, and anomaly detection.

## Problem Statement

Current cost estimation approaches fail in three critical ways:

1. **Accuracy**: Most estimators only calculate compute costs, missing 40-60% of actual bill (data transfer, logs, storage, NAT gateways, etc.)
2. **Staleness**: Hardcoded prices become wrong when AWS changes pricing or user selects different regions
3. **User Expectations**: Users need "what will I pay this month" not "what's the hourly rate"

## User Stories

### US-1: Pre-Deployment Cost Estimate
**As a** user planning an AWS deployment  
**I want to** see an estimated cost range before I deploy  
**So that** I can make informed decisions about whether to proceed

**Acceptance Criteria:**
- AC-1.1: System shows cost range (min-max) not a single point estimate
- AC-1.2: Estimate includes compute, networking, and storage costs
- AC-1.3: Estimate clearly labels what is included and excluded
- AC-1.4: Estimate is region-specific (different prices per region)
- AC-1.5: Estimate shows when pricing data was last fetched
- AC-1.6: Estimate loads in <500ms (from cache, not live API call)

### US-2: Pricing Data Freshness
**As a** platform operator  
**I want** AWS pricing data refreshed automatically  
**So that** estimates remain accurate as AWS changes prices

**Acceptance Criteria:**
- AC-2.1: Background job fetches AWS Pricing API every 24 hours
- AC-2.2: Pricing data stored per region in database
- AC-2.3: Failed fetches retry with exponential backoff
- AC-2.4: System continues using stale data if fetch fails (with warning)
- AC-2.5: Admin can manually trigger pricing refresh
- AC-2.6: Pricing fetch respects AWS API rate limits

### US-3: Post-Deployment Actual Costs
**As a** user with active AWS deployments  
**I want to** see my actual AWS costs after deployment  
**So that** I can track spending and compare to estimates

**Acceptance Criteria:**
- AC-3.1: System fetches actual costs from AWS Cost Explorer API
- AC-3.2: Actual costs shown after 48 hours (AWS Cost Explorer delay)
- AC-3.3: Shows cost-to-date and projected monthly cost
- AC-3.4: Compares actual vs estimated with variance percentage
- AC-3.5: Breaks down costs by service (EC2, S3, RDS, etc.)
- AC-3.6: Updates daily automatically

### US-4: Cost Anomaly Detection
**As a** user with active AWS deployments  
**I want to** be alerted when costs exceed expectations  
**So that** I can prevent surprise bills

**Acceptance Criteria:**
- AC-4.1: Alert triggered when actual cost exceeds estimate by 20%
- AC-4.2: Alert sent via email and in-app notification
- AC-4.3: Alert includes breakdown of which services exceeded budget
- AC-4.4: User can set custom alert thresholds per deployment
- AC-4.5: Alert includes recommended actions (scale down, check logs, etc.)

### US-5: Cost Transparency
**As a** user viewing cost estimates  
**I want** clear explanations of what's included and excluded  
**So that** I understand why actual costs may differ

**Acceptance Criteria:**
- AC-5.1: Estimate shows itemized breakdown (compute, network, storage)
- AC-5.2: Disclaimer clearly states excluded costs (data transfer overages, etc.)
- AC-5.3: Tooltip/help text explains each cost component
- AC-5.4: Shows assumptions (e.g., "Based on 10GB data transfer/month")
- AC-5.5: Link to AWS pricing calculator for detailed analysis

## Technical Requirements

### TR-1: AWS Pricing API Integration
- TR-1.1: Use AWS Price List API (not hardcoded rates)
- TR-1.2: Cache pricing data in SQLite database
- TR-1.3: Support all AWS regions
- TR-1.4: Handle API rate limits gracefully
- TR-1.5: Pricing data versioned (track when fetched)

### TR-2: Cost Calculation Accuracy
- TR-2.1: Include all relevant services for each blueprint:
  - **Static Site**: S3 storage, S3 requests, CloudFront data transfer, Route53
  - **Web App**: Fargate vCPU, Fargate memory, ALB hours, ALB LCUs, RDS instance, RDS storage, NAT gateway, CloudWatch logs, ECR storage
  - **Full-Stack**: All of above + additional services
- TR-2.2: Calculate range with 20% margin (min: estimate * 0.8, max: estimate * 1.4)
- TR-2.3: Use realistic usage assumptions (documented and configurable)
- TR-2.4: Round to 2 decimal places for display

### TR-3: Performance
- TR-3.1: Cost estimate endpoint responds in <500ms
- TR-3.2: Pricing refresh job completes in <5 minutes
- TR-3.3: Cost Explorer fetch completes in <10 seconds
- TR-3.4: Database queries optimized with indexes

### TR-4: Data Model
- TR-4.1: `awsPricingCache` collection stores rates per region/service
- TR-4.2: `costEstimates` collection stores pre-deployment estimates
- TR-4.3: `actualCosts` collection stores post-deployment actuals
- TR-4.4: `costAlerts` collection tracks anomaly alerts

### TR-5: Error Handling
- TR-5.1: Graceful degradation if pricing API unavailable
- TR-5.2: Show warning if pricing data is stale (>48 hours)
- TR-5.3: Retry logic for transient API failures
- TR-5.4: Log all pricing fetch errors for debugging

## Non-Functional Requirements

### NFR-1: Reliability
- System continues functioning with stale pricing data
- No single point of failure in pricing pipeline
- Automatic recovery from API failures

### NFR-2: Accuracy
- Estimates within 30% of actual costs for 90% of deployments
- Clear communication when estimates may be inaccurate
- Regular validation against actual AWS bills

### NFR-3: Transparency
- All assumptions documented and visible to users
- Clear labeling of included/excluded costs
- Pricing data freshness always displayed

### NFR-4: Compliance
- No storage of actual AWS billing data (privacy)
- Cost data isolated per user (security)
- Audit trail of all cost calculations

## Out of Scope (Future Enhancements)

- Multi-cloud cost estimation (GCP, Azure)
- Reserved instance / savings plan recommendations
- Cost optimization suggestions
- Historical cost trending and forecasting
- Budget management and enforcement
- Integration with AWS Budgets API for hard limits

## Success Metrics

1. **Accuracy**: 90% of deployments have actual costs within estimate range
2. **Trust**: <5% of users report surprise bills
3. **Performance**: 99% of estimate requests complete in <500ms
4. **Freshness**: Pricing data never stale >48 hours
5. **Adoption**: 80% of users review cost estimate before deploying

## Dependencies

- AWS Price List API access
- AWS Cost Explorer API access (requires 24-48 hour delay)
- Background job scheduler (cron)
- Database storage for pricing cache
- Email notification system for alerts

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| AWS Pricing API rate limits | High | Cache aggressively, fetch once per 24h |
| Cost Explorer data delay | Medium | Set expectations (48h delay), show estimate until then |
| Regional pricing differences | High | Store prices per region, validate region selection |
| Estimate inaccuracy | High | Show range not point, clear disclaimers, track variance |
| API authentication failures | Medium | Retry logic, fallback to stale data, alert admins |

## Implementation Phases

### Phase 1: Pricing Cache (Week 1)
- Database schema for pricing cache
- Background job to fetch AWS Pricing API
- API endpoint to retrieve cached prices
- Admin UI to view pricing data freshness

### Phase 2: Pre-Deployment Estimates (Week 2)
- Cost calculation engine
- Estimate API endpoint
- Frontend cost estimator component
- Range display with disclaimers

### Phase 3: Post-Deployment Actuals (Week 3)
- AWS Cost Explorer integration
- Actual cost tracking per deployment
- Variance calculation and display
- Cost breakdown by service

### Phase 4: Anomaly Detection (Week 4)
- Alert threshold configuration
- Cost monitoring background job
- Email notification integration
- In-app alert display

## References

- [AWS Price List API Documentation](https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/price-changes.html)
- [AWS Cost Explorer API Documentation](https://docs.aws.amazon.com/cost-management/latest/APIReference/Welcome.html)
- [AWS Pricing Calculator](https://calculator.aws/)
- [Stripe Cost Estimation Best Practices](https://stripe.com/docs/billing/subscriptions/usage-based)

## Glossary

- **Cost Estimate**: Pre-deployment prediction of monthly AWS costs
- **Actual Cost**: Real AWS charges from Cost Explorer API
- **Variance**: Percentage difference between estimate and actual
- **Pricing Cache**: Database storage of AWS service prices per region
- **Cost Range**: Min-max estimate showing uncertainty
- **Anomaly**: Actual cost exceeding estimate by threshold percentage
