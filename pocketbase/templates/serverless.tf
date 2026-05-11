# Serverless Application Blueprint (Lambda + API Gateway)
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

variable "user_id" {
  description = "User ID for resource tagging"
  type        = string
}

variable "project_id" {
  description = "Project ID for resource tagging"
  type        = string
}

variable "deployment_id" {
  description = "Deployment ID for resource tagging"
  type        = string
}

variable "custom_domain" {
  description = "Custom domain for the API"
  type        = string
  default     = ""
}

variable "route53_zone_id" {
  description = "Route53 zone ID for the custom domain"
  type        = string
  default     = ""
}

# Configure AWS Provider
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
  default_tags {
    tags = {
      ManagedBy    = "AutoStack"
      UserId       = var.user_id
      ProjectId    = var.project_id
      DeploymentId = var.deployment_id
      Environment  = "production"
    }
  }
}

# IAM Role for Lambda
resource "aws_iam_role" "lambda" {
  name = "${var.app_name}-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    Name = "${var.app_name}-lambda-role"
  }
}

resource "aws_iam_role_policy_attachment" "lambda_basic" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
  role       = aws_iam_role.lambda.name
}

# CloudWatch Log Group for Lambda
resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${var.app_name}"
  retention_in_days = 7

  tags = {
    Name = "${var.app_name}-lambda-logs"
  }
}

# Placeholder Lambda Function (User will upload code via different mechanism or we provide a default)
# For now, we create a simple "Hello World" function
resource "aws_lambda_function" "main" {
  function_name = var.app_name
  role          = aws_iam_role.lambda.arn
  handler       = "index.handler"
  runtime       = var.runtime

  # Inline code for demonstration
  filename         = "lambda_function_payload.zip"
  source_code_hash = data.archive_file.lambda.output_base64sha256

  tags = {
    Name = var.app_name
  }
}

data "archive_file" "lambda" {
  type        = "zip"
  output_path = "lambda_function_payload.zip"

  source {
    content  = <<EOF
exports.handler = async (event) => {
    const response = {
        statusCode: 200,
        body: JSON.stringify({
            message: 'Hello from AutoStack!',
            app: '${var.app_name}',
            deploymentId: '${var.deployment_id}'
        }),
    };
    return response;
};
EOF
    filename = "index.js"
  }
}

# API Gateway (HTTP API)
resource "aws_apigatewayv2_api" "main" {
  name          = "${var.app_name}-api"
  protocol_type = "HTTP"

  tags = {
    Name = "${var.app_name}-api"
  }
}

resource "aws_apigatewayv2_stage" "main" {
  api_id = aws_apigatewayv2_api.main.id

  name        = "$default"
  auto_deploy = true

  tags = {
    Name = "${var.app_name}-stage"
  }
}

resource "aws_apigatewayv2_integration" "main" {
  api_id = aws_apigatewayv2_api.main.id

  integration_uri    = aws_lambda_function.main.invoke_arn
  integration_type   = "AWS_PROXY"
  integration_method = "POST"
}

resource "aws_apigatewayv2_route" "main" {
  api_id = aws_apigatewayv2_api.main.id

  route_key = "ANY /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.main.id}"
}

resource "aws_apigatewayv2_route" "root" {
  api_id = aws_apigatewayv2_api.main.id

  route_key = "ANY /"
  target    = "integrations/${aws_apigatewayv2_integration.main.id}"
}

resource "aws_lambda_permission" "api_gw" {
  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.main.function_name
  principal     = "apigateway.amazonaws.com"

  source_arn = "${aws_apigatewayv2_api.main.execution_arn}/*/*"
}

# ACM Certificate
resource "aws_acm_certificate" "main" {
  count             = var.custom_domain != "" ? 1 : 0
  domain_name       = var.custom_domain
  validation_method = "DNS"

  tags = {
    Name = "${var.app_name}-cert"
  }

  lifecycle {
    create_before_destroy = true
  }
}

# DNS Validation Record
resource "aws_route53_record" "validation" {
  for_each = var.custom_domain != "" ? {
    for dvo in aws_acm_certificate.main[0].domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  } : {}

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = var.route53_zone_id
}

# Certificate Validation
resource "aws_acm_certificate_validation" "main" {
  count                   = var.custom_domain != "" ? 1 : 0
  certificate_arn         = aws_acm_certificate.main[0].arn
  validation_record_fqdns = [for record in aws_route53_record.validation : record.fqdn]
}

# Custom Domain for API Gateway
resource "aws_apigatewayv2_domain_name" "main" {
  count       = var.custom_domain != "" ? 1 : 0
  domain_name = var.custom_domain

  domain_name_configuration {
    certificate_arn = aws_acm_certificate_validation.main[0].certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

# API Mapping
resource "aws_apigatewayv2_api_mapping" "main" {
  count       = var.custom_domain != "" ? 1 : 0
  api_id      = aws_apigatewayv2_api.main.id
  domain_name = aws_apigatewayv2_domain_name.main[0].id
  stage       = aws_apigatewayv2_stage.main.id
}

# Route53 Alias Record for API Gateway
resource "aws_route53_record" "api" {
  count   = var.custom_domain != "" ? 1 : 0
  zone_id = var.route53_zone_id
  name    = var.custom_domain
  type    = "A"

  alias {
    name                   = aws_apigatewayv2_domain_name.main[0].domain_name_configuration[0].target_domain_name
    zone_id                = aws_apigatewayv2_domain_name.main[0].domain_name_configuration[0].hosted_zone_id
    evaluate_target_health = false
  }
}

# Outputs
output "api_endpoint" {
  description = "API Gateway endpoint URL"
  value       = var.custom_domain != "" ? "https://${var.custom_domain}" : aws_apigatewayv2_api.main.api_endpoint
}

output "lambda_function_name" {
  description = "Lambda function name"
  value       = aws_lambda_function.main.function_name
}

output "lambda_function_arn" {
  description = "Lambda function ARN"
  value       = aws_lambda_function.main.arn
}
