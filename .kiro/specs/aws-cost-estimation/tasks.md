# AWS Cost Estimation - Implementation Tasks

## Phase 1: Pricing Cache Infrastructure (Week 1)

### 1.1 Database Schema Setup
- [x] Create `awsPricingCache` collection with proper indexes
  - **Validates**: TR-4.1 (awsPricingCache collection stores rates per region/service)
  - **Files**: `pocketbase/pb_migrations/[timestamp]_create_aws_pricing_cache.js`
- [x] Create `costEstimates` collection with deployment relationships  
  - **Validates**: TR-4.2 (costEstimates collection stores pre-deployment estimates)
  - **Files**: `pocketbase/pb_migrations/[timestamp]_create_cost_estimates.js`
- [x] Create `actualCosts` collection with time-series structure
  - **Validates**: TR-4.3 (actualCosts collection stores post-deployment actuals)
  - **Files**: `pocketbase/pb_migrations/[timestamp]_create_actual_costs.js`
- [x] Create `costAlerts` collection with user notification tracking
  - **Validates**: TR-4.4 (costAlerts collection tracks anomaly alerts)
  - **Files**: `pocketbase/pb_migrations/[timestamp]_create_cost_alerts.js`
- [x] Add database migration for new collections
  - **Validates**: All TR-4.x requirements
  - **Files**: Migration files above
- [x] Write unit tests for schema validation
  - **Validates**: TR-4.x data integrity
  - **Files**: `pocketbase/pkg/models/cost_test.go`

### 1.2 AWS Pricing API Integration
- [x] Implement `PricingFetcher` service with AWS SDK integration
  - **Validates**: TR-1.1 (Use AWS Price List API)
  - **Files**: `pocketbase/pkg/aws/pricing_fetcher.go`
- [x] Add support for fetching Fargate/ECS pricing by region
  - **Validates**: TR-2.1 (Include all relevant services)
  - **Files**: `pocketbase/pkg/aws/pricing_fargate.go`
- [x] Add support for fetching S3 storage and request pricing
  - **Validates**: TR-2.1 (S3 storage, S3 requests)
  - **Files**: `pocketbase/pkg/aws/pricing_s3.go`
- [x] Add support for fetching ALB and data transfer pricing
  - **Validates**: TR-2.1 (ALB hours, ALB LCUs)
  - **Files**: `pocketbase/pkg/aws/pricing_alb.go`
- [x] Add support for fetching RDS instance pricing
  - **Validates**: TR-2.1 (RDS instance, RDS storage)
  - **Files**: `pocketbase/pkg/aws/pricing_rds.go`
- [x] Implement exponential backoff retry logic for API failures
  - **Validates**: TR-5.3 (Retry logic for transient API failures)
  - **Files**: `pocketbase/pkg/aws/retry_logic.go`
- [x] Write integration tests with AWS API mocks
  - **Validates**: TR-1.4 (Handle API rate limits gracefully)
  - **Files**: `pocketbase/pkg/aws/pricing_fetcher_test.go`

### 1.3 Background Job System
- [x] Create cron job scheduler for pricing updates
  - **Validates**: AC-2.1 (Background job fetches AWS Pricing API every 24 hours)
  - **Files**: `pocketbase/pkg/jobs/pricing_scheduler.go`
- [x] Implement daily pricing fetch job (2 AM UTC)
  - **Validates**: AC-2.1 (Every 24 hours refresh)
  - **Files**: `pocketbase/pkg/jobs/pricing_fetch_job.go`
- [x] Add job monitoring and failure alerting
  - **Validates**: AC-2.3 (Failed fetches retry with exponential backoff)
  - **Files**: `pocketbase/pkg/jobs/job_monitor.go`
- [x] Implement graceful handling of stale pricing data
  - **Validates**: AC-2.4 (System continues using stale data if fetch fails)
  - **Files**: `pocketbase/pkg/aws/stale_data_handler.go`
- [x] Add admin endpoint to manually trigger pricing refresh
  - **Validates**: AC-2.5 (Admin can manually trigger pricing refresh)
  - **Files**: `pocketbase/pkg/controller/admin_pricing.go`
- [x] Write tests for job execution and error scenarios
  - **Validates**: TR-5.1 (Graceful degradation if pricing API unavailable)
  - **Files**: `pocketbase/pkg/jobs/pricing_job_test.go`

### 1.4 Pricing Cache API
- [x] Create `/api/admin/pricing/status` endpoint for cache health
  - **Validates**: AC-2.6 (Pricing fetch respects AWS API rate limits)
  - **Files**: `pocketbase/pkg/controller/pricing_status.go`
- [x] Create `/api/admin/pricing/refresh` endpoint for manual updates
  - **Validates**: AC-2.5 (Admin can manually trigger pricing refresh)
  - **Files**: `pocketbase/pkg/controller/pricing_refresh.go`
- [x] Implement pricing data retrieval by region and service
  - **Validates**: TR-1.3 (Support all AWS regions)
  - **Files**: `pocketbase/pkg/aws/pricing_cache.go`
- [x] Add cache invalidation and refresh mechanisms
  - **Validates**: TR-1.5 (Pricing data versioned)
  - **Files**: `pocketbase/pkg/aws/cache_manager.go`
- [x] Write API tests for pricing cache endpoints
  - **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms)
  - **Files**: `pocketbase/pkg/controller/pricing_test.go`

## Phase 2: Cost Estimation Engine (Week 2)

### 2.1 Service Cost Calculators
- [x] Implement `FargateCostCalculator` with vCPU/memory pricing
  - **Validates**: TR-2.1 (Fargate vCPU, Fargate memory)
  - **Files**: `pocketbase/pkg/cost/fargate_calculator.go`
- [x] Implement `S3CostCalculator` with storage and request pricing
  - **Validates**: TR-2.1 (S3 storage, S3 requests)
  - **Files**: `pocketbase/pkg/cost/s3_calculator.go`
- [x] Implement `ALBCostCalculator` with fixed and LCU-based pricing
  - **Validates**: TR-2.1 (ALB hours, ALB LCUs)
  - **Files**: `pocketbase/pkg/cost/alb_calculator.go`
- [x] Implement `RDSCostCalculator` with instance and storage pricing
  - **Validates**: TR-2.1 (RDS instance, RDS storage)
  - **Files**: `pocketbase/pkg/cost/rds_calculator.go`
- [x] Implement `CloudFrontCostCalculator` with data transfer pricing
  - **Validates**: TR-2.1 (CloudFront data transfer)
  - **Files**: `pocketbase/pkg/cost/cloudfront_calculator.go`
- [x] Implement `Route53CostCalculator` with hosted zone pricing
  - **Validates**: TR-2.1 (Route53)
  - **Files**: `pocketbase/pkg/cost/route53_calculator.go`
- [x] Write unit tests for each calculator with mock pricing data
  - **Validates**: TR-2.4 (Round to 2 decimal places for display)
  - **Files**: `pocketbase/pkg/cost/calculators_test.go`

### 2.2 Blueprint Cost Mapping
- [x] Define cost calculation rules for "static-website" blueprint
  - **Validates**: TR-2.1 (Static Site: S3 storage, S3 requests, CloudFront data transfer, Route53)
  - **Files**: `pocketbase/pkg/cost/blueprint_static_website.go`
- [x] Define cost calculation rules for "web-application" blueprint  
  - **Validates**: TR-2.1 (Web App: Fargate vCPU, memory, ALB, RDS, NAT gateway, CloudWatch logs, ECR)
  - **Files**: `pocketbase/pkg/cost/blueprint_web_app.go`
- [x] Define cost calculation rules for "full-stack-app" blueprint
  - **Validates**: TR-2.1 (Full-Stack: All of above + additional services)
  - **Files**: `pocketbase/pkg/cost/blueprint_full_stack.go`
- [x] Define cost calculation rules for "microservices" blueprint
  - **Validates**: TR-2.1 (Include all relevant services for each blueprint)
  - **Files**: `pocketbase/pkg/cost/blueprint_microservices.go`
- [x] Implement blueprint-to-calculator mapping system
  - **Validates**: TR-2.3 (Use realistic usage assumptions)
  - **Files**: `pocketbase/pkg/cost/blueprint_mapper.go`
- [x] Add configurable usage assumptions per blueprint
  - **Validates**: AC-5.4 (Shows assumptions)
  - **Files**: `pocketbase/pkg/cost/usage_assumptions.go`
- [x] Write tests for blueprint cost calculations
  - **Validates**: TR-2.2 (Calculate range with 20% margin)
  - **Files**: `pocketbase/pkg/cost/blueprint_test.go`

### 2.3 Cost Estimation API
- [x] Create `POST /api/cost/estimate` endpoint
  - **Validates**: AC-1.6 (Estimate loads in <500ms from cache)
  - **Files**: `pocketbase/pkg/controller/cost_estimate.go`
- [x] Implement request validation for blueprint and region
  - **Validates**: AC-1.4 (Estimate is region-specific)
  - **Files**: `pocketbase/pkg/validation/cost_request.go`
- [x] Add cost range calculation (min: estimate * 0.8, max: estimate * 1.4)
  - **Validates**: AC-1.1 (System shows cost range not single point estimate)
  - **Files**: `pocketbase/pkg/cost/range_calculator.go`
- [x] Implement cost breakdown by service category
  - **Validates**: AC-1.2 (Estimate includes compute, networking, and storage costs)
  - **Files**: `pocketbase/pkg/cost/breakdown_generator.go`
- [x] Add usage assumptions and disclaimers to response
  - **Validates**: AC-1.3 (Estimate clearly labels what is included and excluded)
  - **Files**: `pocketbase/pkg/cost/disclaimer_generator.go`
- [x] Implement response caching (1-hour TTL)
  - **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms)
  - **Files**: `pocketbase/pkg/cache/estimate_cache.go`
- [x] Write API integration tests with various blueprints
  - **Validates**: AC-1.5 (Estimate shows when pricing data was last fetched)
  - **Files**: `pocketbase/pkg/controller/cost_estimate_test.go`

### 2.4 Frontend Cost Estimator Component
- [x] Create `CostEstimator.svelte` component
  - **Validates**: AC-5.1 (Estimate shows itemized breakdown)
  - **Files**: `frontend/src/lib/components/cost/CostEstimator.svelte`
- [x] Implement real-time cost updates on blueprint/region change
  - **Validates**: AC-1.6 (Estimate loads in <500ms)
  - **Files**: Component reactive statements
- [x] Add loading states and error handling
  - **Validates**: TR-5.1 (Graceful degradation if pricing API unavailable)
  - **Files**: Component error handling logic
- [x] Display cost range with clear min/max labeling
  - **Validates**: AC-1.1 (System shows cost range not single point estimate)
  - **Files**: Component template and styling
- [x] Show itemized cost breakdown by service
  - **Validates**: AC-5.1 (Estimate shows itemized breakdown)
  - **Files**: Component breakdown display
- [x] Display usage assumptions and disclaimers
  - **Validates**: AC-5.2 (Disclaimer clearly states excluded costs)
  - **Files**: Component disclaimer section
- [x] Add pricing data freshness indicator
  - **Validates**: AC-1.5 (Estimate shows when pricing data was last fetched)
  - **Files**: Component metadata display
- [x] Write component tests with mock API responses
  - **Validates**: AC-5.3 (Tooltip/help text explains each cost component)
  - **Files**: `frontend/src/lib/components/cost/CostEstimator.test.js`

## Phase 3: Actual Cost Tracking (Week 3)

### 3.1 AWS Cost Explorer Integration
- [x] Implement `ActualCostFetcher` service with Cost Explorer SDK
- [x] Add deployment tagging strategy for cost tracking
- [x] Implement cost data fetching with 48-hour delay handling
- [x] Add cost breakdown by AWS service
- [x] Implement monthly cost projection from partial data
- [x] Add variance calculation (actual vs estimate)
- [x] Write integration tests with Cost Explorer API mocks

### 3.2 Cost Tracking Background Jobs
- [x] Create daily job to fetch actual costs for active deployments
- [-] Implement incremental cost updates (only fetch new data)
- [ ] Add job to clean up cost data for destroyed deployments
- [ ] Implement error handling for Cost Explorer API failures
- [ ] Add monitoring for cost data freshness
- [ ] Write tests for cost tracking job execution

### 3.3 Actual Cost API
- [ ] Create `GET /api/cost/actual/{deploymentId}` endpoint
- [ ] Implement user authorization (deployment ownership)
- [ ] Add cost-to-date and projected monthly calculations
- [ ] Implement variance analysis with percentage calculation
- [ ] Add service-level cost breakdown in response
- [ ] Implement response caching (6-hour TTL)
- [ ] Write API tests for actual cost retrieval

### 3.4 Frontend Actual Cost Component
- [ ] Create `ActualCostDisplay.svelte` component
- [ ] Implement cost comparison (actual vs estimate)
- [ ] Add variance display with color coding (green/orange/red)
- [ ] Show service-level cost breakdown
- [ ] Handle 48-hour delay with appropriate messaging
- [ ] Add automatic refresh for cost updates
- [ ] Write component tests for various cost scenarios

## Phase 4: Anomaly Detection & Alerting (Week 4)

### 4.1 Cost Monitoring System
- [ ] Implement `CostMonitor` service for anomaly detection
- [ ] Add configurable alert thresholds (default 20% variance)
- [ ] Implement daily cost anomaly checking job
- [ ] Add alert deduplication (don't spam users)
- [ ] Implement alert acknowledgment system
- [ ] Write tests for anomaly detection logic

### 4.2 Notification System
- [ ] Integrate with existing notification service
- [ ] Create email templates for cost overrun alerts
- [ ] Implement in-app notification display
- [ ] Add user preferences for alert types and thresholds
- [ ] Implement alert history and tracking
- [ ] Write tests for notification delivery

### 4.3 Alert Management API
- [ ] Create `GET /api/cost/alerts` endpoint for user alerts
- [ ] Create `POST /api/cost/alerts/{id}/acknowledge` endpoint
- [ ] Create `PUT /api/cost/alerts/settings` for user preferences
- [ ] Implement alert filtering and pagination
- [ ] Add alert statistics and summary endpoints
- [ ] Write API tests for alert management

### 4.4 Frontend Alert Components
- [ ] Create `CostAlerts.svelte` component for alert display
- [ ] Create `AlertSettings.svelte` for user preferences
- [ ] Add alert notifications to deployment detail pages
- [ ] Implement alert acknowledgment UI
- [ ] Add alert history and statistics views
- [ ] Write component tests for alert interactions

## Phase 5: Testing & Optimization (Week 5)

### 5.1 Performance Testing
- [ ] Load test cost estimation API (1000 concurrent requests)
- [ ] Performance test pricing cache under heavy load
- [ ] Optimize database queries with proper indexing
- [ ] Implement API response caching strategies
- [ ] Test AWS API rate limit handling
- [ ] Write performance benchmarks and monitoring

### 5.2 Integration Testing
- [ ] End-to-end test: estimate → deploy → actual cost → alert
- [ ] Test pricing cache refresh with real AWS APIs
- [ ] Test Cost Explorer integration with real deployments
- [ ] Validate cost calculation accuracy against AWS bills
- [ ] Test error scenarios and graceful degradation
- [ ] Write comprehensive integration test suite

### 5.3 Security Testing
- [ ] Test user authorization for all cost endpoints
- [ ] Validate input sanitization for all parameters
- [ ] Test data isolation between users
- [ ] Audit cost data access and logging
- [ ] Test API rate limiting and abuse prevention
- [ ] Write security test cases

### 5.4 Documentation & Monitoring
- [x] Create API documentation for all cost endpoints
  - **Validates**: All API endpoints documented with examples
  - **Files**: `docs/AWS_COST_ESTIMATION_API.md`
- [x] Document cost calculation methodology and assumptions
  - **Validates**: TR-2.3 (Usage assumptions documented)
  - **Files**: `docs/AWS_COST_CALCULATION_METHODOLOGY.md`
- [x] Create runbooks for common operational issues
  - **Validates**: TR-5.x (Error handling procedures)
  - **Files**: `docs/AWS_COST_ESTIMATION_RUNBOOKS.md`
- [x] Write user guide for cost estimation features
  - **Validates**: AC-5.x (User transparency requirements)
  - **Files**: `docs/AWS_COST_ESTIMATION_USER_GUIDE.md`
- [x] Set up monitoring dashboards for system health
  - **Validates**: NFR-1 (Reliability monitoring)
  - **Files**: `docs/AWS_COST_MONITORING_DASHBOARD.md`
- [x] Document troubleshooting procedures
  - **Validates**: Operational excellence
  - **Files**: Included in runbooks document

## Correctness Properties

### Cost Calculation Properties
1. **Cost Positivity**: All cost estimates must be positive numbers
   - `∀ estimate: CostEstimate → estimate.total > 0`
   - **Validates**: TR-2.4 (Round to 2 decimal places for display)

2. **Range Consistency**: Cost ranges must satisfy min ≤ estimate ≤ max
   - `∀ estimate: CostEstimate → estimate.rangeMin ≤ estimate.total ≤ estimate.rangeMax`
   - **Validates**: AC-1.1 (System shows cost range not single point estimate)

3. **Regional Pricing Consistency**: Same blueprint in same region produces same estimate
   - `∀ blueprint, region → estimate(blueprint, region) = estimate(blueprint, region)`
   - **Validates**: AC-1.4 (Estimate is region-specific)

4. **Breakdown Completeness**: Service breakdown sums to total estimate
   - `∀ estimate → sum(estimate.breakdown) = estimate.total`
   - **Validates**: AC-1.2 (Estimate includes compute, networking, and storage costs)

5. **Variance Calculation Accuracy**: Variance formula is mathematically correct
   - `variance = (actual - estimate) / estimate * 100`
   - **Validates**: AC-3.4 (Compares actual vs estimated with variance percentage)

### API Response Properties
6. **Response Completeness**: All cost API responses contain required fields
   - `∀ response → hasFields(response, [total, range, breakdown, assumptions, disclaimer])`
   - **Validates**: AC-1.3 (Estimate clearly labels what is included and excluded)

7. **Timestamp Validity**: All timestamps are valid ISO 8601 format
   - `∀ timestamp → isValidISO8601(timestamp)`
   - **Validates**: AC-1.5 (Estimate shows when pricing data was last fetched)

8. **Authorization Consistency**: Users can only access their own deployment costs
   - `∀ user, deployment → canAccess(user, deployment) ↔ deployment.owner = user.id`
   - **Validates**: TR-5.4 (Cost data isolated per user)

### Data Integrity Properties
9. **Pricing Cache Freshness**: Pricing data is never older than 48 hours without warning
   - `∀ pricingData → age(pricingData) ≤ 48h ∨ hasWarning(pricingData)`
   - **Validates**: TR-5.2 (Show warning if pricing data is stale)

10. **Cost Monotonicity**: Actual costs are monotonically increasing over time
    - `∀ deployment, t1, t2 → t1 < t2 → actualCost(deployment, t1) ≤ actualCost(deployment, t2)`
    - **Validates**: AC-3.3 (Shows cost-to-date and projected monthly cost)

## Property-Based Testing Tasks

### PBT-1: Cost Calculation Properties
- [x] **Property 1**: Cost estimates are always positive numbers
  - **Validates**: Cost Calculation Property 1
  - **Files**: `pocketbase/pkg/cost/properties_test.go`
- [x] **Property 2**: Cost ranges have min ≤ estimate ≤ max
  - **Validates**: Cost Calculation Property 2
  - **Files**: `pocketbase/pkg/cost/range_properties_test.go`
- [x] **Property 3**: Regional pricing variations are consistent
  - **Validates**: Cost Calculation Property 3
  - **Files**: `pocketbase/pkg/cost/regional_properties_test.go`
- [ ] **Property 4**: Blueprint cost calculations are deterministic
  - **Validates**: Cost Calculation Property 4
  - **Files**: `pocketbase/pkg/cost/blueprint_properties_test.go`
- [ ] **Property 5**: Variance calculations are mathematically correct
  - **Validates**: Cost Calculation Property 5
  - **Files**: `pocketbase/pkg/cost/variance_properties_test.go`

### PBT-2: API Response Properties  
- [ ] **Property 6**: All cost API responses have required fields
  - **Validates**: API Response Property 6
  - **Files**: `pocketbase/pkg/controller/api_properties_test.go`
- [ ] **Property 7**: Cost values are properly formatted (2 decimal places)
  - **Validates**: TR-2.4 (Round to 2 decimal places for display)
  - **Files**: `pocketbase/pkg/controller/format_properties_test.go`
- [ ] **Property 8**: Timestamps are valid and in correct format
  - **Validates**: API Response Property 7
  - **Files**: `pocketbase/pkg/controller/timestamp_properties_test.go`
- [ ] **Property 9**: User can only access their own deployment costs
  - **Validates**: API Response Property 8
  - **Files**: `pocketbase/pkg/controller/auth_properties_test.go`
- [ ] **Property 10**: API responses are consistent for same inputs
  - **Validates**: Cost Calculation Property 3
  - **Files**: `pocketbase/pkg/controller/consistency_properties_test.go`

### PBT-3: Data Integrity Properties
- [ ] **Property 11**: Pricing cache data is never corrupted
  - **Validates**: Data Integrity Property 9
  - **Files**: `pocketbase/pkg/aws/cache_properties_test.go`
- [ ] **Property 12**: Cost estimates can be reproduced from same inputs
  - **Validates**: Cost Calculation Property 3
  - **Files**: `pocketbase/pkg/cost/reproducibility_properties_test.go`
- [ ] **Property 13**: Actual costs are monotonically increasing over time
  - **Validates**: Data Integrity Property 10
  - **Files**: `pocketbase/pkg/cost/monotonicity_properties_test.go`
- [ ] **Property 14**: Alert thresholds are respected in all scenarios
  - **Validates**: AC-4.1 (Alert triggered when actual cost exceeds estimate by 20%)
  - **Files**: `pocketbase/pkg/alerts/threshold_properties_test.go`
- [ ] **Property 15**: Database constraints are never violated
  - **Validates**: TR-4.x (Data model integrity)
  - **Files**: `pocketbase/pkg/models/constraints_properties_test.go`

## Acceptance Criteria Validation

Each task must validate against the original acceptance criteria:

- **AC-1.1**: Cost range display (min-max) ✓
- **AC-1.2**: Comprehensive cost inclusion ✓  
- **AC-1.3**: Clear labeling and disclaimers ✓
- **AC-1.4**: Region-specific pricing ✓
- **AC-1.5**: Pricing data freshness display ✓
- **AC-1.6**: <500ms response time ✓
- **AC-2.1**: 24-hour pricing refresh ✓
- **AC-2.2**: Regional pricing storage ✓
- **AC-2.3**: Retry logic implementation ✓
- **AC-2.4**: Stale data handling ✓
- **AC-2.5**: Manual refresh capability ✓
- **AC-2.6**: Rate limit compliance ✓
- **AC-3.1**: Cost Explorer integration ✓
- **AC-3.2**: 48-hour delay handling ✓
- **AC-3.3**: Cost projection ✓
- **AC-3.4**: Variance comparison ✓
- **AC-3.5**: Service breakdown ✓
- **AC-3.6**: Daily updates ✓
- **AC-4.1**: 20% threshold alerts ✓
- **AC-4.2**: Multi-channel notifications ✓
- **AC-4.3**: Service-level breakdown ✓
- **AC-4.4**: Custom thresholds ✓
- **AC-4.5**: Actionable recommendations ✓
- **AC-5.1**: Itemized breakdown ✓
- **AC-5.2**: Clear disclaimers ✓
- **AC-5.3**: Help documentation ✓
- **AC-5.4**: Assumption transparency ✓
- **AC-5.5**: External calculator links ✓

## Success Metrics Tracking

- [ ] Implement accuracy tracking (90% within estimate range)
- [ ] Monitor user trust metrics (<5% surprise bills)
- [ ] Track performance metrics (99% <500ms response)
- [ ] Monitor data freshness (never >48h stale)
- [ ] Measure adoption rates (80% estimate usage)

This task breakdown provides a clear implementation path for enterprise-grade AWS cost estimation that builds user trust through accuracy and transparency.