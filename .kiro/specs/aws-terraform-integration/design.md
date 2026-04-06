# Design Document: AWS Terraform Integration

## Overview

This document outlines the technical design for integrating Terraform-based AWS infrastructure provisioning into the AutoStack platform. The design extends the existing Kubernetes deployment architecture to support AWS as an alternative deployment target while maintaining consistency in user experience and system architecture.

## Architecture Overview

### High-Level Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Frontend      │    │   Backend       │    │   AWS Cloud     │
│   (SvelteKit)   │    │   (Go/PocketBase│    │                 │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ Deployment  │ │    │ │ Terraform   │ │    │ │ ECS/Fargate │ │
│ │ Target      │◄┼────┼►│ Executor    │◄┼────┼►│ ALB/RDS     │ │
│ │ Selector    │ │    │ │ Service     │ │    │ │ S3/Lambda   │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ AWS         │ │    │ │ Credential  │ │    │ │ S3 State    │ │
│ │ Deployment  │ │    │ │ Manager     │ │    │ │ Backend     │ │
│ │ UI          │ │    │ │             │ │    │ │             │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Component Architecture

The design follows the existing AutoStack patterns with new AWS-specific components:

1. **Frontend Extensions**: New UI components for AWS deployment management
2. **Backend Services**: Terraform executor and AWS-specific controllers
3. **Database Schema**: New collections for AWS deployments and state management
4. **Infrastructure**: Terraform templates and AWS resource provisioning

## Database Design

### New Collections

#### awsDeployments
```json
{
  "id": "string (15 chars)",
  "name": "string",
  "project": "relation(projects)",
  "user": "relation(users)", 
  "blueprint": "relation(awsBlueprints)",
  "region": "string",
  "status": "string", // pending, planning, applying, active, failed, destroying, destroyed
  "configuration": "json", // user inputs (instance_type, db_config, env_vars, etc.)
  "outputs": "json", // terraform outputs (urls, endpoints, resource_ids)
  "created": "datetime",
  "updated": "datetime"
}
```

#### awsRollouts
```json
{
  "id": "string (15 chars)",
  "deployment": "relation(awsDeployments)",
  "project": "relation(projects)",
  "user": "relation(users)",
  "terraformConfig": "text", // generated terraform configuration
  "stateVersion": "string", // S3 state file version
  "startDate": "datetime",
  "endDate": "datetime",
  "status": "string", // planning, applying, active, failed, destroying, destroyed
  "planOutput": "text", // terraform plan output
  "executionLogs": "text", // terraform execution logs
  "created": "datetime",
  "updated": "datetime"
}
```

#### awsBlueprints
```json
{
  "id": "string",
  "name": "string",
  "description": "text",
  "category": "string", // web-app, full-stack, static-site, serverless
  "terraformTemplate": "text", // terraform template with variables
  "configSchema": "json", // JSON schema for user configuration
  "supportedRegions": "json", // array of supported AWS regions
  "estimatedCost": "json", // base cost estimates by resource type
  "logo": "file",
  "owner": "relation(users)",
  "public": "bool",
  "created": "datetime",
  "updated": "datetime"
}
```

#### awsCredentials
```json
{
  "id": "string",
  "user": "relation(users)",
  "accessKeyId": "string", // encrypted
  "secretAccessKey": "string", // encrypted
  "sessionToken": "string", // encrypted, optional
  "region": "string", // default region
  "validated": "bool",
  "lastValidated": "datetime",
  "created": "datetime",
  "updated": "datetime"
}
```

#### terraformExecutions
```json
{
  "id": "string",
  "rollout": "relation(awsRollouts)",
  "operation": "string", // init, plan, apply, destroy
  "status": "string", // running, completed, failed
  "output": "text", // command output
  "exitCode": "number",
  "startTime": "datetime",
  "endTime": "datetime",
  "created": "datetime"
}
```

## Backend Design

### Terraform Executor Service

```go
package terraform

type ExecutorService struct {
    workingDir    string
    stateBackend  StateBackend
    credManager   CredentialManager
    logger        Logger
}

type ExecutionResult struct {
    ExitCode    int
    Output      string
    Error       error
    Duration    time.Duration
}

func (e *ExecutorService) Init(deploymentId string) (*ExecutionResult, error)
func (e *ExecutorService) Plan(deploymentId string, config TerraformConfig) (*ExecutionResult, error)
func (e *ExecutorService) Apply(deploymentId string, autoApprove bool) (*ExecutionResult, error)
func (e *ExecutorService) Destroy(deploymentId string, autoApprove bool) (*ExecutionResult, error)
func (e *ExecutorService) GetOutputs(deploymentId string) (map[string]interface{}, error)
```

### State Management

```go
package state

type S3StateBackend struct {
    bucket    string
    keyPrefix string
    region    string
    client    *s3.Client
}

func (s *S3StateBackend) InitializeBackend(deploymentId string) error
func (s *S3StateBackend) GetStateVersion(deploymentId string) (string, error)
func (s *S3StateBackend) RestoreState(deploymentId, version string) error
func (s *S3StateBackend) ListVersions(deploymentId string) ([]StateVersion, error)
```

### Credential Management

```go
package credentials

type CredentialManager struct {
    encryptionKey []byte
    db           *pocketbase.PocketBase
}

func (c *CredentialManager) StoreCredentials(userId string, creds AWSCredentials) error
func (c *CredentialManager) GetCredentials(userId string) (*AWSCredentials, error)
func (c *CredentialManager) ValidateCredentials(creds AWSCredentials) error
func (c *CredentialManager) EncryptValue(value string) (string, error)
func (c *CredentialManager) DecryptValue(encrypted string) (string, error)
```

### Cost Estimation Service

```go
package cost

type EstimatorService struct {
    pricingClient *pricing.Client
    cache        map[string]PricingData
}

func (e *EstimatorService) EstimateCost(blueprint Blueprint, config DeploymentConfig) (*CostEstimate, error)
func (e *EstimatorService) GetResourcePricing(region, service, instanceType string) (*ResourcePricing, error)
```

## Frontend Design

### Component Structure

```
src/lib/components/aws/
├── deployment/
│   ├── AWSDeploymentCard.svelte
│   ├── NewAWSDeployment.svelte
│   ├── AWSDeploymentTabs.svelte
│   ├── InfrastructureOverview.svelte
│   ├── TerraformLogs.svelte
│   ├── OutputsDisplay.svelte
│   └── InfrastructureDiagram.svelte
├── blueprints/
│   ├── AWSBlueprintCard.svelte
│   ├── BlueprintSelector.svelte
│   └── ConfigurationForm.svelte
├── credentials/
│   ├── AWSCredentialsSetup.svelte
│   └── CredentialValidator.svelte
└── shared/
    ├── DeploymentTargetSelector.svelte
    ├── RegionSelector.svelte
    ├── CostEstimator.svelte
    └── StatusIndicator.svelte
```

### Key UI Components

#### DeploymentTargetSelector.svelte
```svelte
<script lang="ts">
  export let selectedTarget: 'kubernetes' | 'aws' = 'kubernetes';
  
  const targets = [
    { id: 'kubernetes', name: 'Kubernetes', icon: 'kubernetes-icon' },
    { id: 'aws', name: 'AWS', icon: 'aws-icon' }
  ];
</script>

<div class="deployment-target-selector">
  {#each targets as target}
    <button 
      class="target-option {selectedTarget === target.id ? 'selected' : ''}"
      on:click={() => selectedTarget = target.id}
    >
      <Icon name={target.icon} />
      {target.name}
    </button>
  {/each}
</div>
```

#### NewAWSDeployment.svelte
```svelte
<script lang="ts">
  import { awsBlueprints, awsCredentials } from '$lib/stores/aws';
  import CostEstimator from './CostEstimator.svelte';
  import RegionSelector from './RegionSelector.svelte';
  
  let deploymentName = '';
  let selectedBlueprint: AWSBlueprint;
  let selectedRegion = 'us-east-1';
  let configuration = {};
  let costEstimate: CostEstimate;
  
  async function handleDeploy() {
    // Create AWS deployment
    const deployment = await createAWSDeployment({
      name: deploymentName,
      blueprint: selectedBlueprint.id,
      region: selectedRegion,
      configuration
    });
    
    // Start terraform execution
    await startTerraformExecution(deployment.id);
  }
</script>
```

### State Management

```typescript
// src/lib/stores/aws.ts
import { writable } from 'svelte/store';

export const awsDeployments = writable<AWSDeployment[]>([]);
export const awsBlueprints = writable<AWSBlueprint[]>([]);
export const awsCredentials = writable<AWSCredentials | null>(null);
export const selectedAWSDeployment = writable<AWSDeployment | null>(null);

export interface AWSDeployment {
  id: string;
  name: string;
  status: DeploymentStatus;
  region: string;
  outputs: Record<string, any>;
  configuration: DeploymentConfiguration;
  created: string;
  updated: string;
}
```

## Terraform Blueprint Templates

### ECS Web App Blueprint

```hcl
# templates/ecs-web-app.tf
variable "app_name" {
  description = "Application name"
  type        = string
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "instance_type" {
  description = "ECS task instance type"
  type        = string
  default     = "256"
}

variable "container_image" {
  description = "Docker container image"
  type        = string
}

variable "environment_variables" {
  description = "Environment variables for the container"
  type        = map(string)
  default     = {}
}

# VPC and Networking
resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
  
  tags = {
    Name         = "${var.app_name}-vpc"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

# ECS Cluster
resource "aws_ecs_cluster" "main" {
  name = "${var.app_name}-cluster"
  
  tags = {
    Name         = "${var.app_name}-cluster"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

# Application Load Balancer
resource "aws_lb" "main" {
  name               = "${var.app_name}-alb"
  internal           = false
  load_balancer_type = "application"
  subnets            = aws_subnet.public[*].id
  security_groups    = [aws_security_group.alb.id]
  
  tags = {
    Name         = "${var.app_name}-alb"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

# ECS Service
resource "aws_ecs_service" "main" {
  name            = "${var.app_name}-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.main.arn
  desired_count   = 1
  launch_type     = "FARGATE"
  
  network_configuration {
    subnets         = aws_subnet.private[*].id
    security_groups = [aws_security_group.ecs.id]
  }
  
  load_balancer {
    target_group_arn = aws_lb_target_group.main.arn
    container_name   = var.app_name
    container_port   = 80
  }
  
  tags = {
    Name         = "${var.app_name}-service"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

# Outputs
output "application_url" {
  description = "Application URL"
  value       = "http://${aws_lb.main.dns_name}"
}

output "cluster_name" {
  description = "ECS cluster name"
  value       = aws_ecs_cluster.main.name
}

output "load_balancer_dns" {
  description = "Load balancer DNS name"
  value       = aws_lb.main.dns_name
}
```

### Full Stack Blueprint (ECS + RDS)

```hcl
# templates/full-stack-app.tf
# ... (includes all ECS resources from above plus:)

# RDS Database
resource "aws_db_instance" "main" {
  identifier     = "${var.app_name}-db"
  engine         = var.db_engine
  engine_version = var.db_engine_version
  instance_class = var.db_instance_class
  
  allocated_storage     = var.db_allocated_storage
  max_allocated_storage = var.db_max_allocated_storage
  
  db_name  = var.db_name
  username = var.db_username
  password = var.db_password
  
  vpc_security_group_ids = [aws_security_group.rds.id]
  db_subnet_group_name   = aws_db_subnet_group.main.name
  
  backup_retention_period = 7
  backup_window          = "03:00-04:00"
  maintenance_window     = "sun:04:00-sun:05:00"
  
  skip_final_snapshot = true
  
  tags = {
    Name         = "${var.app_name}-db"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

# Additional outputs
output "database_endpoint" {
  description = "RDS database endpoint"
  value       = aws_db_instance.main.endpoint
  sensitive   = true
}

output "database_port" {
  description = "RDS database port"
  value       = aws_db_instance.main.port
}
```

## Security Design

### Credential Encryption

```go
package security

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
)

type EncryptionService struct {
    key []byte
}

func (e *EncryptionService) Encrypt(plaintext string) (string, error) {
    block, err := aes.NewCipher(e.key)
    if err != nil {
        return "", err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := rand.Read(nonce); err != nil {
        return "", err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}
```

### IAM Policy Template

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecs:*",
        "ec2:*",
        "elasticloadbalancing:*",
        "rds:*",
        "s3:*",
        "iam:PassRole",
        "logs:*",
        "cloudwatch:*"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:RequestedRegion": ["us-east-1", "us-west-2", "eu-west-1"]
        }
      }
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject"
      ],
      "Resource": "arn:aws:s3:::autostack-terraform-state/*"
    }
  ]
}
```

## API Design

### REST Endpoints

```
# AWS Deployments
POST   /api/aws/deployments
GET    /api/aws/deployments
GET    /api/aws/deployments/:id
PUT    /api/aws/deployments/:id
DELETE /api/aws/deployments/:id

# AWS Rollouts
POST   /api/aws/deployments/:id/rollouts
GET    /api/aws/deployments/:id/rollouts
GET    /api/aws/rollouts/:id
PUT    /api/aws/rollouts/:id

# Terraform Operations
POST   /api/aws/deployments/:id/plan
POST   /api/aws/deployments/:id/apply
POST   /api/aws/deployments/:id/destroy
GET    /api/aws/deployments/:id/outputs
GET    /api/aws/deployments/:id/logs

# AWS Blueprints
GET    /api/aws/blueprints
POST   /api/aws/blueprints
GET    /api/aws/blueprints/:id
PUT    /api/aws/blueprints/:id
DELETE /api/aws/blueprints/:id

# Cost Estimation
POST   /api/aws/cost-estimate

# Credentials
POST   /api/aws/credentials
GET    /api/aws/credentials/validate
PUT    /api/aws/credentials
DELETE /api/aws/credentials
```

### WebSocket Events

```typescript
// Real-time terraform execution logs
interface TerraformLogEvent {
  type: 'terraform_log';
  deploymentId: string;
  rolloutId: string;
  operation: 'init' | 'plan' | 'apply' | 'destroy';
  logLevel: 'info' | 'warn' | 'error';
  message: string;
  timestamp: string;
}

// Deployment status updates
interface DeploymentStatusEvent {
  type: 'deployment_status';
  deploymentId: string;
  status: DeploymentStatus;
  outputs?: Record<string, any>;
}
```

## Error Handling

### Terraform Error Classification

```go
type TerraformError struct {
    Type        ErrorType
    Message     string
    Resource    string
    Suggestion  string
}

type ErrorType string

const (
    ValidationError    ErrorType = "validation"
    AuthenticationError ErrorType = "authentication"
    ResourceError      ErrorType = "resource"
    NetworkError       ErrorType = "network"
    StateError         ErrorType = "state"
    UnknownError       ErrorType = "unknown"
)

func ClassifyTerraformError(output string) *TerraformError {
    // Parse terraform output and classify errors
    // Provide user-friendly suggestions
}
```

## Performance Considerations

### Terraform Execution Optimization

1. **Parallel Execution**: Use Terraform's built-in parallelism
2. **State Caching**: Cache frequently accessed state information
3. **Resource Tagging**: Efficient resource filtering and querying
4. **Log Streaming**: Chunked log streaming to prevent memory issues

### Database Optimization

1. **Indexing**: Index frequently queried fields (user_id, project_id, status)
2. **Archival**: Archive old rollouts and execution logs
3. **Connection Pooling**: Efficient database connection management

## Monitoring and Observability

### Metrics Collection

```go
type Metrics struct {
    TerraformExecutions    counter
    DeploymentDuration     histogram
    ErrorRate             gauge
    ActiveDeployments     gauge
    CostEstimateAccuracy  histogram
}
```

### Health Checks

1. **Terraform Binary**: Verify terraform is available and functional
2. **AWS Connectivity**: Test AWS API connectivity
3. **S3 State Backend**: Verify state backend accessibility
4. **Database**: Check database connectivity and performance

## Migration Strategy

### Phase 1: Core Infrastructure
- Implement basic Terraform executor
- Add AWS credential management
- Create simple ECS blueprint

### Phase 2: UI Integration
- Add deployment target selector
- Implement AWS deployment forms
- Add real-time log streaming

### Phase 3: Advanced Features
- Add cost estimation
- Implement infrastructure visualization
- Add rollback functionality

### Phase 4: Production Readiness
- Add comprehensive error handling
- Implement audit logging
- Add performance monitoring

## Correctness Properties

### Property 1: Credential Security
**Validates: Requirements 3.1, 3.2, 3.3**
```
∀ credentials ∈ AWSCredentials:
  stored(credentials) → encrypted(credentials) ∧ 
  access(credentials) → authenticated(user) ∧
  isolated(credentials, user)
```

### Property 2: State Consistency
**Validates: Requirements 8.1, 8.6**
```
∀ deployment ∈ AWSDeployments:
  ∀ rollout ∈ deployment.rollouts:
    active(rollout) → ∃! state ∈ TerraformStates:
      references(rollout, state) ∧ consistent(state, infrastructure)
```

### Property 3: Resource Isolation
**Validates: Requirements 14.1, 14.3**
```
∀ resource ∈ AWSResources:
  ∀ user1, user2 ∈ Users:
    user1 ≠ user2 → 
    ¬accessible(resource, user1) ∨ ¬accessible(resource, user2)
```

### Property 4: Deployment Atomicity
**Validates: Requirements 6.4, 6.6**
```
∀ deployment ∈ AWSDeployments:
  apply(deployment) → 
    (∀ resources ∈ deployment.resources: created(resources)) ∨
    (∀ resources ∈ deployment.resources: ¬created(resources))
```

### Property 5: Cost Estimate Accuracy
**Validates: Requirements 5.1, 5.4**
```
∀ estimate ∈ CostEstimates:
  ∀ deployment ∈ AWSDeployments:
    generated(estimate, deployment) →
    |actual_cost(deployment) - estimate.total| ≤ 0.2 * estimate.total
```

This design provides a comprehensive foundation for implementing AWS Terraform integration while maintaining consistency with the existing AutoStack architecture and ensuring security, reliability, and user experience standards.