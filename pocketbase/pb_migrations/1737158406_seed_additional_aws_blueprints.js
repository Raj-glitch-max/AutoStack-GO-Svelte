/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db);
  
  // Full Stack Application Blueprint
  const fullStackCollection = dao.findCollectionByNameOrId("awsBlueprints");
  const fullStackRecord = new Record(fullStackCollection, {
    "id": "full_stack_app_001",
    "name": "Full Stack Application",
    "description": "Complete web application with ECS Fargate, Application Load Balancer, and RDS PostgreSQL database",
    "category": "full-stack",
    "terraformTemplate": `# Full Stack Application Blueprint (ECS + RDS)
# This template creates a complete web application infrastructure
# including ECS Fargate, ALB, RDS, VPC, and all necessary networking

variable "app_name" {
  description = "Application name"
  type        = string
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "container_image" {
  description = "Docker container image"
  type        = string
}

variable "instance_type" {
  description = "ECS task CPU/Memory"
  type        = string
  default     = "512"
}

variable "db_password" {
  description = "Database password"
  type        = string
  sensitive   = true
}

# VPC and networking resources
resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
  
  tags = {
    Name         = "\${var.app_name}-vpc"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

# ECS Cluster and Service
resource "aws_ecs_cluster" "main" {
  name = "\${var.app_name}-cluster"
}

# RDS Database
resource "aws_db_instance" "main" {
  identifier     = "\${var.app_name}-db"
  engine         = "postgres"
  engine_version = "15.4"
  instance_class = "db.t3.micro"
  
  allocated_storage = 20
  db_name          = "appdb"
  username         = "dbuser"
  password         = var.db_password
  
  skip_final_snapshot = true
}

# Outputs
output "application_url" {
  description = "Application URL"
  value       = "http://example-alb.amazonaws.com"
}

output "database_endpoint" {
  description = "Database endpoint"
  value       = aws_db_instance.main.endpoint
  sensitive   = true
}`,
    "configSchema": JSON.stringify({
      "type": "object",
      "properties": {
        "app_name": {
          "type": "string",
          "title": "Application Name",
          "description": "Name for your application"
        },
        "container_image": {
          "type": "string",
          "title": "Container Image",
          "description": "Docker image to deploy",
          "default": "nginx:latest"
        },
        "instance_type": {
          "type": "string",
          "title": "Instance Size",
          "enum": ["512", "1024", "2048"],
          "enumNames": ["Medium (0.5 vCPU, 1 GB)", "Large (1 vCPU, 2 GB)", "X-Large (2 vCPU, 4 GB)"],
          "default": "512"
        },
        "db_password": {
          "type": "string",
          "title": "Database Password",
          "description": "Password for the PostgreSQL database",
          "format": "password",
          "minLength": 8
        }
      },
      "required": ["app_name", "container_image", "db_password"]
    }),
    "supportedRegions": JSON.stringify([
      "us-east-1", "us-east-2", "us-west-1", "us-west-2",
      "eu-west-1", "eu-west-2", "eu-central-1",
      "ap-southeast-1", "ap-southeast-2", "ap-northeast-1"
    ]),
    "estimatedCost": JSON.stringify({
      "compute": 25,
      "database": 15,
      "networking": 10,
      "storage": 3,
      "total": 53
    }),
    "owner": "system",
    "public": true
  });
  
  // Static Website Blueprint
  const staticSiteRecord = new Record(fullStackCollection, {
    "id": "static_site_001",
    "name": "Static Website",
    "description": "Fast and scalable static website hosting with S3, CloudFront CDN, and optional custom domain",
    "category": "static-site",
    "terraformTemplate": `# Static Website Blueprint (S3 + CloudFront)
# This template creates a static website hosting solution
# with S3 for storage and CloudFront for global CDN

variable "app_name" {
  description = "Application name"
  type        = string
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "domain_name" {
  description = "Custom domain name (optional)"
  type        = string
  default     = ""
}

# S3 Bucket for website hosting
resource "aws_s3_bucket" "website" {
  bucket = "\${var.app_name}-\${random_id.bucket_suffix.hex}"
  
  tags = {
    Name         = "\${var.app_name}-website"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

resource "random_id" "bucket_suffix" {
  byte_length = 4
}

# CloudFront Distribution
resource "aws_cloudfront_distribution" "website" {
  origin {
    domain_name = aws_s3_bucket.website.bucket_regional_domain_name
    origin_id   = "S3-\${aws_s3_bucket.website.id}"
  }

  enabled             = true
  default_root_object = "index.html"

  default_cache_behavior {
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    target_origin_id       = "S3-\${aws_s3_bucket.website.id}"
    viewer_protocol_policy = "redirect-to-https"
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }
}

# Outputs
output "website_url" {
  description = "Website URL"
  value       = "https://\${aws_cloudfront_distribution.website.domain_name}"
}

output "s3_bucket_name" {
  description = "S3 bucket name"
  value       = aws_s3_bucket.website.id
}`,
    "configSchema": JSON.stringify({
      "type": "object",
      "properties": {
        "app_name": {
          "type": "string",
          "title": "Website Name",
          "description": "Name for your static website"
        },
        "domain_name": {
          "type": "string",
          "title": "Custom Domain (Optional)",
          "description": "Your custom domain name (e.g., www.example.com)",
          "format": "hostname"
        }
      },
      "required": ["app_name"]
    }),
    "supportedRegions": JSON.stringify([
      "us-east-1", "us-east-2", "us-west-1", "us-west-2",
      "eu-west-1", "eu-west-2", "eu-central-1",
      "ap-southeast-1", "ap-southeast-2", "ap-northeast-1"
    ]),
    "estimatedCost": JSON.stringify({
      "storage": 0.5,
      "cdn": 1.0,
      "requests": 0.5,
      "total": 2.0
    }),
    "owner": "system",
    "public": true
  });
  
  // Serverless API Blueprint
  const serverlessRecord = new Record(fullStackCollection, {
    "id": "serverless_api_001",
    "name": "Serverless API",
    "description": "Scalable serverless API with AWS Lambda, API Gateway, and DynamoDB",
    "category": "serverless",
    "terraformTemplate": `# Serverless API Blueprint (Lambda + API Gateway + DynamoDB)
# This template creates a serverless API infrastructure
# with Lambda functions, API Gateway, and DynamoDB

variable "app_name" {
  description = "Application name"
  type        = string
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "runtime" {
  description = "Lambda runtime"
  type        = string
  default     = "nodejs18.x"
}

# DynamoDB Table
resource "aws_dynamodb_table" "main" {
  name           = "\${var.app_name}-table"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "id"

  attribute {
    name = "id"
    type = "S"
  }

  tags = {
    Name         = "\${var.app_name}-table"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

# Lambda Function
resource "aws_lambda_function" "api" {
  filename         = "function.zip"
  function_name    = "\${var.app_name}-api"
  role            = aws_iam_role.lambda.arn
  handler         = "index.handler"
  runtime         = var.runtime

  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.main.name
    }
  }

  tags = {
    Name         = "\${var.app_name}-lambda"
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    ProjectId    = var.project_id
    DeploymentId = var.deployment_id
  }
}

# API Gateway
resource "aws_api_gateway_rest_api" "main" {
  name        = "\${var.app_name}-api"
  description = "API for \${var.app_name}"
}

# Outputs
output "api_url" {
  description = "API Gateway URL"
  value       = "\${aws_api_gateway_rest_api.main.execution_arn}"
}

output "table_name" {
  description = "DynamoDB table name"
  value       = aws_dynamodb_table.main.name
}`,
    "configSchema": JSON.stringify({
      "type": "object",
      "properties": {
        "app_name": {
          "type": "string",
          "title": "API Name",
          "description": "Name for your serverless API"
        },
        "runtime": {
          "type": "string",
          "title": "Lambda Runtime",
          "enum": ["nodejs18.x", "python3.9", "python3.10", "java11", "dotnet6"],
          "enumNames": ["Node.js 18.x", "Python 3.9", "Python 3.10", "Java 11", ".NET 6"],
          "default": "nodejs18.x"
        }
      },
      "required": ["app_name"]
    }),
    "supportedRegions": JSON.stringify([
      "us-east-1", "us-east-2", "us-west-1", "us-west-2",
      "eu-west-1", "eu-west-2", "eu-central-1",
      "ap-southeast-1", "ap-southeast-2", "ap-northeast-1"
    ]),
    "estimatedCost": JSON.stringify({
      "lambda": 5.0,
      "apigateway": 3.0,
      "dynamodb": 2.0,
      "total": 10.0
    }),
    "owner": "system",
    "public": true
  });
  
  dao.saveRecord(fullStackRecord);
  dao.saveRecord(staticSiteRecord);
  dao.saveRecord(serverlessRecord);

  // Return success (no specific return value needed for successful migrations)
}, (db) => {
  const dao = new Dao(db);
  
  // Delete the seeded blueprints
  const blueprintIds = ["full_stack_app_001", "static_site_001", "serverless_api_001"];
  
  blueprintIds.forEach(id => {
    try {
      const record = dao.findRecordById("awsBlueprints", id);
      dao.deleteRecord(record);
    } catch (error) {
      console.log(`Blueprint ${id} not found, skipping deletion`);
    }
  });
  
  // Return success (no specific return value needed for successful migrations)
})