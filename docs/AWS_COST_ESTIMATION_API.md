# AWS Cost Estimation API Documentation

## Overview

The AWS Cost Estimation API provides endpoints for retrieving cost estimates, actual costs, pricing data, and cost alerts for AWS deployments. All endpoints require authentication and follow RESTful conventions.

## Base URL

```
/api
```

## Authentication

All endpoints require a valid PocketBase authentication token passed in the `Authorization` header:

```
Authorization: Bearer <token>
```

## Endpoints

### Cost Estimation

#### POST /api/cost/estimate

Get a cost estimate for a specific blueprint and region before deployment.

**Request Body:**
```json
{
  "blueprintId": "string",
  "region": "string",
  "configuration": {
    "instanceType": "string (optional)",
    "storageSize": "number (optional)",
    "dataTransferGB": "number (optional)"
  }
}
```

**Response (200 OK):**
```json
{
  "estimateId": "string",
  "blueprintId": "string",
  "region": "string",
  "currency": "USD",
  "total": 125.50,
  "rangeMin": 100.40,
  "rangeMax": 175.70,
  "breakdown": {
    "compute": {
      "service": "Fargate",
      "cost": 75.00,
      "details": "2 vCPU, 4GB memory, 730 hours/month"
    },
    "networking": {
      "service": "ALB",
      "cost": 25.00,
      "details": "1 ALB, 10 LCUs average"
    },
    "storage": {
      "service": "RDS",
      "cost": 25.50,
      "details": "db.t3.micro, 20GB storage"
    }
  },
  "assumptions": [
    "Based on 730 hours/month runtime",
    "10GB data transfer per month",
    "Standard storage class",
    "Single availability zone"
  ],
  "disclaimer": "Estimate excludes: data transfer overages, CloudWatch detailed monitoring, backup storage, and support costs. Actual costs may vary based on usage patterns.",
  "pricingDataFetchedAt": "2026-04-10T02:00:00Z",
  "calculatedAt": "2026-04-11T10:30:00Z"
}
```

**Error Responses:**
- `400 Bad Request`: Invalid blueprint or region
- `401 Unauthorized`: Missing or invalid authentication token
- `404 Not Found`: Blueprint not found
- `500 Internal Server Error`: Pricing data unavailable
- `503 Service Unavailable`: Pricing data is stale (>48 hours)

**Performance:**
- Target response time: <500ms
- Cached for 1 hour per blueprint/region combination

---

#### GET /api/cost/actual/{deploymentId}

Get actual AWS costs for a deployed application.

**Path Parameters:**
- `deploymentId`: The deployment ID

**Query Parameters:**
- `period`: Time period (optional, default: "month")
  - Values: "day", "week", "month", "all"

**Response (200 OK):**
```json
{
  "deploymentId": "string",
  "region": "string",
  "currency": "USD",
  "costToDate": 142.75,
  "projectedMonthly": 155.00,
  "estimatedMonthly": 125.50,
  "variance": 13.5,
  "variancePercentage": 23.5,
  "breakdown": [
    {
      "service": "Fargate",
      "cost": 82.00,
      "percentage": 57.5
    },
    {
      "service": "ALB",
      "cost": 28.50,
      "percentage": 20.0
    },
    {
      "service": "RDS",
      "cost": 27.25,
      "percentage": 19.1
    },
    {
      "service": "CloudWatch",
      "cost": 5.00,
      "percentage": 3.5
    }
  ],
  "dataFetchedAt": "2026-04-11T06:00:00Z",
  "periodStart": "2026-04-01T00:00:00Z",
  "periodEnd": "2026-04-11T00:00:00Z",
  "note": "Cost data has 48-hour delay from AWS Cost Explorer"
}
```

**Error Responses:**
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: User does not own this deployment
- `404 Not Found`: Deployment not found
- `425 Too Early`: Cost data not yet available (within 48 hours of deployment)
- `500 Internal Server Error`: AWS Cost Explorer API error

**Performance:**
- Target response time: <2s
- Cached for 6 hours

---

### Pricing Data Management

#### GET /api/admin/pricing/status

Get the status of the pricing cache (admin only).

**Response (200 OK):**
```json
{
  "lastFetchedAt": "2026-04-10T02:00:00Z",
  "nextScheduledFetch": "2026-04-11T02:00:00Z",
  "status": "healthy",
  "regions": [
    {
      "region": "us-east-1",
      "services": 15,
      "lastUpdated": "2026-04-10T02:00:00Z"
    },
    {
      "region": "eu-west-1",
      "services": 15,
      "lastUpdated": "2026-04-10T02:00:00Z"
    }
  ],
  "totalPricePoints": 450,
  "cacheSize": "2.5MB",
  "isStale": false,
  "warnings": []
}
```

**Error Responses:**
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: User is not an admin

---

#### POST /api/admin/pricing/refresh

Manually trigger a pricing data refresh (admin only).

**Request Body:**
```json
{
  "regions": ["us-east-1", "eu-west-1"],
  "force": false
}
```

**Response (202 Accepted):**
```json
{
  "jobId": "string",
  "status": "queued",
  "message": "Pricing refresh job queued",
  "estimatedDuration": "3-5 minutes"
}
```

**Error Responses:**
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: User is not an admin
- `429 Too Many Requests`: Refresh already in progress

---

### Cost Alerts

#### GET /api/cost/alerts

Get cost alerts for the authenticated user.

**Query Parameters:**
- `status`: Filter by status (optional)
  - Values: "active", "acknowledged", "resolved"
- `deploymentId`: Filter by deployment (optional)
- `limit`: Number of results (default: 50, max: 100)
- `offset`: Pagination offset (default: 0)

**Response (200 OK):**
```json
{
  "alerts": [
    {
      "id": "string",
      "deploymentId": "string",
      "deploymentName": "My Web App",
      "type": "cost_overrun",
      "severity": "warning",
      "status": "active",
      "message": "Actual cost exceeds estimate by 23.5%",
      "actualCost": 142.75,
      "estimatedCost": 125.50,
      "variance": 17.25,
      "variancePercentage": 23.5,
      "threshold": 20.0,
      "breakdown": [
        {
          "service": "Fargate",
          "variance": 9.3
        }
      ],
      "recommendations": [
        "Review Fargate task sizing - may be over-provisioned",
        "Check for unexpected traffic spikes in ALB metrics",
        "Consider enabling auto-scaling to optimize costs"
      ],
      "createdAt": "2026-04-11T08:00:00Z",
      "acknowledgedAt": null
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

**Error Responses:**
- `401 Unauthorized`: Missing or invalid authentication token

---

#### POST /api/cost/alerts/{alertId}/acknowledge

Acknowledge a cost alert.

**Path Parameters:**
- `alertId`: The alert ID

**Request Body:**
```json
{
  "note": "Investigating the cost increase (optional)"
}
```

**Response (200 OK):**
```json
{
  "id": "string",
  "status": "acknowledged",
  "acknowledgedAt": "2026-04-11T10:30:00Z",
  "acknowledgedBy": "user@example.com"
}
```

**Error Responses:**
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: User does not own this alert
- `404 Not Found`: Alert not found

---

#### PUT /api/cost/alerts/settings

Update cost alert settings for the authenticated user.

**Request Body:**
```json
{
  "defaultThreshold": 20.0,
  "emailNotifications": true,
  "inAppNotifications": true,
  "deploymentSettings": [
    {
      "deploymentId": "string",
      "threshold": 15.0,
      "enabled": true
    }
  ]
}
```

**Response (200 OK):**
```json
{
  "userId": "string",
  "defaultThreshold": 20.0,
  "emailNotifications": true,
  "inAppNotifications": true,
  "updatedAt": "2026-04-11T10:30:00Z"
}
```

**Error Responses:**
- `400 Bad Request`: Invalid threshold value (must be 0-100)
- `401 Unauthorized`: Missing or invalid authentication token

---

## Rate Limits

- Cost estimate endpoint: 100 requests per minute per user
- Actual cost endpoint: 50 requests per minute per user
- Admin endpoints: 10 requests per minute per admin
- Alert endpoints: 100 requests per minute per user

## Error Response Format

All error responses follow this format:

```json
{
  "error": {
    "code": "string",
    "message": "string",
    "details": {}
  }
}
```

## Webhooks (Future)

Webhook support for cost alerts is planned for a future release.

## SDK Examples

### JavaScript/TypeScript

```typescript
import PocketBase from 'pocketbase';

const pb = new PocketBase('https://your-instance.com');

// Authenticate
await pb.collection('users').authWithPassword('user@example.com', 'password');

// Get cost estimate
const estimate = await pb.send('/api/cost/estimate', {
  method: 'POST',
  body: {
    blueprintId: 'bp_123',
    region: 'us-east-1'
  }
});

console.log(`Estimated monthly cost: $${estimate.total}`);
console.log(`Range: $${estimate.rangeMin} - $${estimate.rangeMax}`);

// Get actual costs
const actual = await pb.send(`/api/cost/actual/${deploymentId}`, {
  method: 'GET'
});

console.log(`Actual cost to date: $${actual.costToDate}`);
console.log(`Variance: ${actual.variancePercentage}%`);
```

### cURL

```bash
# Get cost estimate
curl -X POST https://your-instance.com/api/cost/estimate \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "blueprintId": "bp_123",
    "region": "us-east-1"
  }'

# Get actual costs
curl https://your-instance.com/api/cost/actual/dep_123 \
  -H "Authorization: Bearer YOUR_TOKEN"

# Get alerts
curl https://your-instance.com/api/cost/alerts?status=active \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Changelog

### Version 1.0.0 (2026-04-11)
- Initial release
- Cost estimation endpoint
- Actual cost tracking
- Cost alerts
- Admin pricing management

## Support

For API support, please contact the platform team or file an issue in the repository.
