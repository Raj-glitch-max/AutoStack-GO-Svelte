# AutoStack API Documentation

Complete API reference for AutoStack.

## Base URL

```
http://localhost:8090/api
```

## Authentication

All API endpoints require authentication using PocketBase tokens.

```bash
# Login
curl -X POST http://localhost:8090/api/collections/users/auth-with-password \
  -H "Content-Type: application/json" \
  -d '{"identity": "user@example.com", "password": "password"}'

# Use token in subsequent requests
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8090/api/endpoint
```

## Endpoints

### Cost Estimation

#### POST /api/aws/cost-estimate

Estimate AWS deployment costs using Infracost.

**Request:**
```json
{
  "blueprint": "static-site",
  "region": "us-east-1",
  "terraform_code": "optional terraform code"
}
```

**Response:**
```json
{
  "totalMonthlyCost": 2.50,
  "breakdown": {
    "S3 Storage": 0.50,
    "CloudFront": 1.50,
    "Route53": 0.50
  },
  "currency": "USD",
  "region": "us-east-1",
  "disclaimer": "Estimate provided by Infracost..."
}
```

### AWS Deployments

#### POST /api/aws/deployments

Create a new AWS deployment.

**Request:**
```json
{
  "name": "my-app",
  "blueprint": "web-application",
  "region": "us-east-1",
  "variables": {
    "instance_type": "t3.micro",
    "storage_size": 20
  }
}
```

**Response:**
```json
{
  "id": "deployment_123",
  "status": "pending",
  "created": "2026-04-12T10:00:00Z"
}
```

#### GET /api/aws/deployments/:id

Get deployment status and details.

**Response:**
```json
{
  "id": "deployment_123",
  "name": "my-app",
  "status": "running",
  "url": "https://my-app.example.com",
  "cost": {
    "estimated": 32.50,
    "actual": 35.20
  }
}
```

#### DELETE /api/aws/deployments/:id

Destroy an AWS deployment.

**Response:**
```json
{
  "message": "Deployment destruction initiated",
  "status": "destroying"
}
```

### Kubernetes Deployments

#### GET /pb/:projectId/:deploymentId/status

Get Kubernetes deployment status.

**Response:**
```json
{
  "status": "running",
  "replicas": 3,
  "availableReplicas": 3,
  "pods": [
    {
      "name": "my-app-abc123",
      "status": "Running",
      "restarts": 0
    }
  ]
}
```

#### GET /pb/:projectId/:podName/logs

Get pod logs.

**Response:**
```
2026-04-12 10:00:00 [INFO] Application started
2026-04-12 10:00:01 [INFO] Listening on port 8080
```

### Projects

#### GET /api/collections/projects/records

List all projects.

**Response:**
```json
{
  "items": [
    {
      "id": "project_123",
      "name": "Production",
      "namespace": "prod",
      "created": "2026-04-12T10:00:00Z"
    }
  ]
}
```

### Blueprints

#### GET /pb/blueprints/:blueprintId

Get blueprint details.

**Response:**
```json
{
  "id": "blueprint_123",
  "name": "Static Website",
  "description": "S3 + CloudFront static site",
  "services": ["S3", "CloudFront", "Route53"],
  "estimatedCost": 2.50
}
```

## WebSocket Endpoints

### Deployment Logs

```javascript
const ws = new WebSocket('ws://localhost:8090/ws/aws/logs');
ws.onmessage = (event) => {
  const log = JSON.parse(event.data);
  console.log(log.message);
};
```

### Deployment Status

```javascript
const ws = new WebSocket('ws://localhost:8090/ws/aws/status');
ws.onmessage = (event) => {
  const status = JSON.parse(event.data);
  console.log(status.state);
};
```

## Error Responses

All errors follow this format:

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "Blueprint not found",
    "details": {}
  }
}
```

### Error Codes

| Code | Description |
|------|-------------|
| `INVALID_REQUEST` | Invalid request parameters |
| `UNAUTHORIZED` | Missing or invalid authentication |
| `FORBIDDEN` | Insufficient permissions |
| `NOT_FOUND` | Resource not found |
| `RATE_LIMIT` | Too many requests |
| `SERVER_ERROR` | Internal server error |

## Rate Limits

- Cost estimation: 100 requests/minute
- Deployments: 10 requests/minute
- Other endpoints: 1000 requests/minute

## SDKs

### JavaScript/TypeScript

```typescript
import PocketBase from 'pocketbase';

const pb = new PocketBase('http://localhost:8090');

// Authenticate
await pb.collection('users').authWithPassword('user@example.com', 'password');

// Estimate cost
const estimate = await pb.send('/api/aws/cost-estimate', {
  method: 'POST',
  body: {
    blueprint: 'static-site',
    region: 'us-east-1'
  }
});
```

### cURL

```bash
# Authenticate
TOKEN=$(curl -X POST http://localhost:8090/api/collections/users/auth-with-password \
  -H "Content-Type: application/json" \
  -d '{"identity":"user@example.com","password":"password"}' \
  | jq -r '.token')

# Estimate cost
curl -X POST http://localhost:8090/api/aws/cost-estimate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"blueprint":"static-site","region":"us-east-1"}'
```

## Changelog

### v1.0.0 (2026-04-12)
- Initial API release
- Cost estimation endpoint
- AWS deployment endpoints
- Kubernetes deployment endpoints
- WebSocket support for real-time updates
