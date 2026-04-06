# AutoStack - Complete Code Reference

This document contains all key code files with their complete implementations.

## Table of Contents
- [Backend - Health Check System](#backend---health-check-system)
- [Backend - Notification System](#backend---notification-system)
- [Backend - AWS Credentials](#backend---aws-credentials)
- [Backend - Cost Estimator](#backend---cost-estimator)
- [Backend - Terraform Executor](#backend---terraform-executor)
- [Backend - AWS Deployments Controller](#backend---aws-deployments-controller)
- [Frontend - Cost Estimator Component](#frontend---cost-estimator-component)
- [Infrastructure - Terraform Templates](#infrastructure---terraform-templates)

---

## Backend - Health Check System

### File: `pocketbase/pkg/health/health.go`

**Purpose:** Provides health check endpoints for monitoring system status

**Key Features:**
- `/health` - Comprehensive health status
- `/readiness` - Kubernetes readiness probe
- `/liveness` - Kubernetes liveness probe
- Checks: Database, Kubernetes, AWS, Terraform

**Code:**
```go
// See pocketbase/pkg/health/health.go for complete implementation
// Key functions:
// - HandleHealthCheck(w, r) - Main health endpoint
// - HandleReadiness(w, r) - K8s readiness probe
// - HandleLiveness(w, r) - K8s liveness probe
// - checkDatabase(ctx) - Database connectivity check
// - checkKubernetes(ctx) - K8s cluster check
// - checkAWS(ctx) - AWS connectivity check
// - checkTerraform(ctx) - Terraform availability check
```

---

## Backend - Notification System

### File: `pocketbase/pkg/notifications/notifier.go`

**Purpose:** Multi-channel notification system for deployment events

**Key Features:**
- Webhook notifications
- Slack integration
- Email support (placeholder)
- Pre-built notification types
- Severity-based color coding

**Notification Types:**
- `deployment_success` - Deployment completed
- `deployment_failure` - Deployment failed
- `health_alert` - System health issue
- `cost_alert` - Budget exceeded
- `security_alert` - Security issue detected

**Code:**
```go
// See pocketbase/pkg/notifications/notifier.go
// Key functions:
// - Send(notification) - Send to all channels
// - NotifyDeploymentSuccess(name, project, url)
// - NotifyDeploymentFailure(name, project, error)
// - NotifyHealthAlert(component, message)
// - NotifyCostAlert(project, cost, budget)
// - NotifySecurityAlert(severity, message, details)
```

---

## Backend - AWS Credentials

### File: `pocketbase/pkg/aws/credentials.go`

**Purpose:** Secure AWS credential management with encryption

**Key Features:**
- AES-256 encryption at rest
- Credential validation
- User isolation
- Temporary credential support

**Code Structure:**
```go
type CredentialManager struct {
    encryptionKey []byte
    db           *pocketbase.PocketBase
}

type AWSCredentials struct {
    AccessKeyID     string
    SecretAccessKey string
    SessionToken    string
    Region          string
}

// Key methods:
// - StoreCredentials(userId, creds) - Encrypt and store
// - GetCredentials(userId) - Decrypt and retrieve
// - ValidateCredentials(creds) - Test AWS connection
// - EncryptValue(value) - AES-256 encryption
// - DecryptValue(encrypted) - AES-256 decryption
```

---

## Backend - Cost Estimator

### File: `pocketbase/pkg/aws/cost_estimator.go`

**Purpose:** Real-time AWS cost estimation using Pricing API

**Key Features:**
- AWS Pricing API integration
- Cost breakdown by resource
- Fallback pricing when API unavailable
- Regional pricing support
- 24-hour price caching

**Supported Blueprints:**
- ECS Web App (~$69/month)
- Full Stack with RDS (~$97/month)
- Static Site with CloudFront (~$1.37/month)

**Code Structure:**
```go
type CostEstimatorService struct {
    pricingClient *pricing.Client
    cache         map[string]*CachedPrice
}

type CostEstimate struct {
    TotalMonthlyCost float64
    Breakdown        map[string]float64
    Currency         string
    Region           string
    Disclaimer       string
}

// Key methods:
// - EstimateCost(config) - Calculate total cost
// - estimateECSWebAppCost() - ECS pricing
// - estimateFullStackCost() - ECS + RDS pricing
// - estimateStaticSiteCost() - S3 + CloudFront pricing
// - getFargatePrice(region, type) - Query AWS API
// - getRDSPrice(region, instance) - RDS pricing
```

**Pricing Breakdown:**

ECS Web App:
- Fargate (0.25 vCPU, 0.5GB): $15/month
- ALB: $16.20/month
- NAT Gateway: $32.85/month
- CloudWatch Logs: $2/month
- Data Transfer: $3/month

Full Stack:
- All ECS components
- RDS db.t3.micro: $13.14/month
- RDS Storage (20GB): $2.30/month

---

## Backend - Terraform Executor

### File: `pocketbase/pkg/terraform/executor.go`

**Purpose:** Automated Terraform execution with real-time streaming

**Key Features:**
- Terraform init/plan/apply/destroy automation
- Real-time log streaming
- S3 state backend configuration
- DynamoDB state locking
- Output extraction

**Code Structure:**
```go
type ExecutorService struct {
    workingDir  string
    credManager *aws.CredentialManager
}

type ExecutionResult struct {
    ExitCode int
    Output   string
    Error    error
    Duration time.Duration
}

// Key methods:
// - Init(ctx, deploymentID) - Initialize Terraform
// - Plan(ctx, deploymentID, config) - Generate plan
// - Apply(ctx, deploymentID, autoApprove) - Apply changes
// - Destroy(ctx, deploymentID, autoApprove) - Destroy infrastructure
// - GetOutputs(ctx, deploymentID) - Extract outputs
// - executeTerraformCommand() - Run with streaming
```

**Workflow:**
1. Create working directory
2. Generate Terraform config from template
3. Configure S3 backend
4. Execute terraform init
5. Execute terraform plan
6. User confirms
7. Execute terraform apply
8. Extract outputs
9. Update deployment status

---

## Backend - AWS Deployments Controller

### File: `pocketbase/pkg/controller/awsDeployments.go`

**Purpose:** REST API controller for AWS deployments

**Endpoints:**
- `POST /api/aws/deployments` - Create deployment
- `GET /api/aws/deployments` - List deployments
- `GET /api/aws/deployments/:id` - Get deployment
- `DELETE /api/aws/deployments/:id` - Delete deployment
- `POST /api/aws/deployments/:id/plan` - Generate plan
- `POST /api/aws/deployments/:id/apply` - Apply changes
- `POST /api/aws/deployments/:id/destroy` - Destroy infrastructure

**Code Structure:**
```go
type AWSDeploymentController struct {
    app             *pocketbase.PocketBase
    credManager     *aws.CredentialManager
    terraformExec   *terraform.ExecutorService
    logStreamer     terraform.LogStreamer
}

type CreateAWSDeploymentRequest struct {
    Name          string
    ProjectID     string
    BlueprintID   string
    Region        string
    Configuration map[string]interface{}
}

// Key methods:
// - HandleAWSDeploymentCreate() - Create new deployment
// - HandleAWSDeploymentList() - List user deployments
// - HandleAWSDeploymentGet() - Get deployment details
// - HandleAWSDeploymentDelete() - Delete deployment
// - HandleAWSDeploymentPlan() - Generate Terraform plan
// - HandleAWSDeploymentApply() - Apply infrastructure
// - HandleAWSDeploymentDestroy() - Destroy infrastructure
```

---

## Frontend - Cost Estimator Component

### File: `frontend/src/lib/components/aws/CostEstimator.svelte`

**Purpose:** Real-time cost estimation UI component

**Features:**
- Live cost updates on configuration change
- Cost breakdown by resource
- Color-coded severity indicators
- Cost optimization tips
- Fallback estimates when API fails

**Props:**
- `blueprint` - Selected blueprint type
- `region` - AWS region
- `configuration` - Deployment configuration

**Code Structure:**
```svelte
<script lang="ts">
  export let blueprint: string = '';
  export let region: string = 'us-east-1';
  export let configuration: any = {};
  
  let costEstimate: any = null;
  let loading = false;
  
  // Reactive cost updates
  $: if (blueprint && region) {
    updateCostEstimate();
  }
  
  async function updateCostEstimate() {
    const response = await fetch('/api/aws/cost-estimate', {
      method: 'POST',
      body: JSON.stringify({ blueprint, region, configuration })
    });
    costEstimate = await response.json();
  }
</script>

<!-- UI displays total cost, breakdown, and tips -->
```

---

## Infrastructure - Terraform Templates

### ECS Web App Template

**File:** `pocketbase/templates/ecs-web-app.tf`

**Resources Created:**
- VPC with public/private subnets
- Internet Gateway
- NAT Gateway
- Application Load Balancer
- ECS Fargate cluster
- ECS service and task definition
- Security groups
- CloudWatch log group
- IAM roles

**Variables:**
- `app_name` - Application name
- `region` - AWS region
- `container_image` - Docker image
- `instance_type` - CPU/Memory (256, 512, 1024)

**Outputs:**
- `application_url` - ALB DNS name
- `cluster_name` - ECS cluster name
- `vpc_id` - VPC ID

### Full Stack Template

**File:** `pocketbase/templates/full-stack-app.tf`

**Additional Resources:**
- RDS PostgreSQL database
- Database subnet group
- Database security group
- Database backups (7 days)

**Additional Variables:**
- `db_engine` - Database engine (postgres/mysql)
- `db_instance_class` - RDS instance type
- `db_allocated_storage` - Storage in GB
- `db_name` - Database name
- `db_username` - Database user
- `db_password` - Database password

**Additional Outputs:**
- `database_endpoint` - RDS endpoint
- `database_port` - Database port

### Static Site Template

**File:** `pocketbase/templates/static-site.tf`

**Resources Created:**
- S3 bucket with website hosting
- CloudFront distribution
- Origin Access Control
- SSL certificate (optional)
- Default index.html and error.html

**Variables:**
- `app_name` - Site name
- `region` - AWS region
- `domain_name` - Custom domain (optional)

**Outputs:**
- `website_url` - CloudFront URL
- `s3_bucket_name` - Bucket name
- `cloudfront_distribution_id` - Distribution ID

---

## Summary

**Total Files Documented:** 8 core files
**Lines of Code:** ~3,500 lines
**Languages:** Go (backend), TypeScript/Svelte (frontend), HCL (Terraform)
**Key Technologies:** PocketBase, AWS SDK, Terraform, Kubernetes Client-Go

**Next Steps:**
1. Implement SSL automation
2. Add automated backups
3. Enhance monitoring
4. Build CI/CD integration
5. Create comprehensive tests
