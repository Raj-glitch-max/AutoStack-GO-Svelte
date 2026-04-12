# Cost Estimate Endpoint Integration Guide

## Overview

The `POST /api/cost/estimate` endpoint provides comprehensive AWS cost estimation for various blueprint types. This endpoint validates AC-1.6 (Estimate loads in <500ms from cache) and integrates with the BlueprintMapper to return detailed cost breakdowns.

## Implementation Details

### Files Created

1. **pocketbase/pkg/controller/cost_estimate.go** - Main controller implementation
2. **pocketbase/pkg/controller/cost_estimate_test.go** - Comprehensive test suite

### Controller Features

The `CostEstimateController` provides three main endpoints:

1. **POST /api/cost/estimate** - Calculate cost estimate for a blueprint
2. **GET /api/cost/blueprints** - List all supported blueprints with details
3. **GET /api/cost/blueprints/:type** - Get detailed information about a specific blueprint

## Integration Steps

### 1. Register Routes in main.go

Add the following code to the `OnBeforeServe` hook in `pocketbase/main.go`:

```go
// Cost Estimation Routes
costEstimateController := controller.NewCostEstimateController(app)
costEstimateController.RegisterCostEstimateRoutes(e.Router)
```

This should be added after the AWS Deployment Routes section, around line 120.

### 2. Example Usage

#### Request Format

```bash
curl -X POST http://localhost:8090/api/cost/estimate \
  -H "Content-Type: application/json" \
  -d '{
    "blueprint": "static-website",
    "region": "us-east-1",
    "variables": {
      "storage_gb": 10,
      "requests_per_month": 10000,
      "data_transfer_gb": 100
    }
  }'
```

#### Response Format

```json
{
  "estimate": {
    "total": 12.45,
    "range": {
      "min": 9.96,
      "max": 17.43
    },
    "breakdown": {
      "storage": 2.30,
      "networking": 9.00,
      "requests": 1.15
    },
    "assumptions": {
      "storage": "10 GB",
      "requests": "10,000 per month",
      "dataTransfer": "100 GB per month",
      "cacheHitRatio": "80%"
    },
    "disclaimer": "Excludes: Data transfer overages beyond 100GB, CloudWatch detailed monitoring...",
    "pricingFetchedAt": "2026-04-11T10:00:00Z",
    "region": "us-east-1",
    "blueprint": "static-website",
    "currency": "USD"
  }
}
```

## Supported Blueprints

The endpoint supports the following blueprint types:

1. **static-website** - S3 + CloudFront static site hosting
2. **web-application** - Fargate + RDS web application
3. **full-stack-app** - Complete full-stack application with frontend and backend
4. **microservices** - Microservices architecture with multiple services

## Validation

The endpoint performs comprehensive validation:

- **Required fields**: `blueprint` and `region` must be provided
- **Blueprint validation**: Blueprint type must be supported
- **Region validation**: Region must be valid AWS region
- **Configuration validation**: Blueprint-specific configuration is validated

## Performance

The endpoint is designed to meet AC-1.6 requirements:

- **Target**: <500ms response time
- **Implementation**: Uses cached pricing data from `awsPricingCache` collection
- **Monitoring**: Logs warning if response time exceeds 500ms

## Error Handling

The endpoint handles various error scenarios:

- **400 Bad Request**: Missing required fields or invalid configuration
- **404 Not Found**: Blueprint type not found (for GET endpoints)
- **500 Internal Server Error**: Calculation failures or database errors

## Testing

Run the test suite:

```bash
cd pocketbase
go test -v ./pkg/controller -run TestEstimateCost
```

### Test Coverage

The test suite includes:

- ✅ Successful cost estimation
- ✅ Missing blueprint validation
- ✅ Missing region validation
- ✅ Unsupported blueprint handling
- ✅ Performance validation (<500ms)
- ✅ All blueprint types
- ✅ Blueprint listing
- ✅ Blueprint details retrieval

## Acceptance Criteria Validation

This implementation validates the following acceptance criteria:

- **AC-1.1**: ✅ System shows cost range (min-max) not single point estimate
- **AC-1.2**: ✅ Estimate includes compute, networking, and storage costs
- **AC-1.3**: ✅ Estimate clearly labels what is included and excluded
- **AC-1.4**: ✅ Estimate is region-specific
- **AC-1.5**: ✅ Estimate shows when pricing data was last fetched
- **AC-1.6**: ✅ Estimate loads in <500ms from cache

## Next Steps

1. **Register routes** in main.go as described above
2. **Test the endpoint** using the provided curl examples
3. **Integrate with frontend** using the CostEstimator.svelte component (Phase 2.4)
4. **Monitor performance** to ensure <500ms response times

## Notes

- The endpoint uses the existing BlueprintMapper and calculator infrastructure
- Cost ranges are calculated as min = estimate * 0.8, max = estimate * 1.4
- Pricing data freshness is retrieved from the awsPricingCache collection
- The endpoint is designed to work with the existing pricing cache system
