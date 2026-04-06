/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db);
  
  // Use inline Terraform template
  const terraformTemplate = `# ECS Web Application Blueprint
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
  default     = "256"
}

# VPC and Networking
resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "\${var.app_name}-vpc"
  }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "\${var.app_name}-igw"
  }
}

resource "aws_subnet" "public" {
  count             = 2
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.0.\${count.index + 1}.0/24"
  availability_zone = data.aws_availability_zones.available.names[count.index]

  map_public_ip_on_launch = true

  tags = {
    Name = "\${var.app_name}-public-\${count.index + 1}"
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = {
    Name = "\${var.app_name}-public-rt"
  }
}

resource "aws_route_table_association" "public" {
  count          = 2
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# Security Groups
resource "aws_security_group" "alb" {
  name_prefix = "\${var.app_name}-alb-"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "\${var.app_name}-alb-sg"
  }
}

resource "aws_security_group" "ecs" {
  name_prefix = "\${var.app_name}-ecs-"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port       = 80
    to_port         = 80
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "\${var.app_name}-ecs-sg"
  }
}

# Application Load Balancer
resource "aws_lb" "main" {
  name               = "\${var.app_name}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id

  tags = {
    Name = "\${var.app_name}-alb"
  }
}

resource "aws_lb_target_group" "main" {
  name        = "\${var.app_name}-tg"
  port        = 80
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip"

  health_check {
    enabled             = true
    healthy_threshold   = 2
    interval            = 30
    matcher             = "200"
    path                = "/"
    port                = "traffic-port"
    protocol            = "HTTP"
    timeout             = 5
    unhealthy_threshold = 2
  }

  tags = {
    Name = "\${var.app_name}-tg"
  }
}

resource "aws_lb_listener" "main" {
  load_balancer_arn = aws_lb.main.arn
  port              = "80"
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.main.arn
  }
}

# ECS Cluster
resource "aws_ecs_cluster" "main" {
  name = var.app_name

  tags = {
    Name = "\${var.app_name}-cluster"
  }
}

# ECS Task Definition
resource "aws_ecs_task_definition" "main" {
  family                   = var.app_name
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.instance_type
  memory                   = var.instance_type == "256" ? "512" : var.instance_type == "512" ? "1024" : "2048"
  execution_role_arn       = aws_iam_role.ecs_execution.arn

  container_definitions = jsonencode([
    {
      name  = var.app_name
      image = var.container_image
      portMappings = [
        {
          containerPort = 80
          protocol      = "tcp"
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.main.name
          "awslogs-region"        = var.region
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])

  tags = {
    Name = "\${var.app_name}-task"
  }
}

# ECS Service
resource "aws_ecs_service" "main" {
  name            = var.app_name
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.main.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    security_groups = [aws_security_group.ecs.id]
    subnets         = aws_subnet.public[*].id
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.main.arn
    container_name   = var.app_name
    container_port   = 80
  }

  depends_on = [aws_lb_listener.main]

  tags = {
    Name = "\${var.app_name}-service"
  }
}

# IAM Role for ECS Execution
resource "aws_iam_role" "ecs_execution" {
  name = "\${var.app_name}-ecs-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    Name = "\${var.app_name}-ecs-execution-role"
  }
}

resource "aws_iam_role_policy_attachment" "ecs_execution" {
  role       = aws_iam_role.ecs_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# CloudWatch Log Group
resource "aws_cloudwatch_log_group" "main" {
  name              = "/ecs/\${var.app_name}"
  retention_in_days = 7

  tags = {
    Name = "\${var.app_name}-logs"
  }
}

# Outputs
output "application_url" {
  description = "Application URL"
  value       = "http://\${aws_lb.main.dns_name}"
}

output "load_balancer_dns" {
  description = "Load Balancer DNS Name"
  value       = aws_lb.main.dns_name
}`;
  
  // Create the blueprint record
  const collection = dao.findCollectionByNameOrId("awsBlueprints");
  const record = new Record(collection, {
    "id": "ecs_web_app_001",
    "name": "ECS Web Application",
    "description": "Deploy a containerized web application using AWS ECS Fargate with Application Load Balancer",
    "category": "web-app",
    "terraformTemplate": terraformTemplate,
    "configSchema": JSON.stringify({
      "type": "object",
      "properties": {
        "app_name": {
          "type": "string",
          "title": "Application Name",
          "description": "Name for your application (used in resource naming)"
        },
        "container_image": {
          "type": "string",
          "title": "Container Image",
          "description": "Docker image to deploy (e.g., nginx:latest)",
          "default": "nginx:latest"
        },
        "instance_type": {
          "type": "string",
          "title": "Instance Size",
          "enum": ["256", "512", "1024"],
          "enumNames": ["Small (0.25 vCPU, 0.5 GB)", "Medium (0.5 vCPU, 1 GB)", "Large (1 vCPU, 2 GB)"],
          "default": "256"
        }
      },
      "required": ["app_name", "container_image"]
    }),
    "supportedRegions": JSON.stringify([
      "us-east-1", "us-east-2", "us-west-1", "us-west-2",
      "eu-west-1", "eu-west-2", "eu-central-1",
      "ap-southeast-1", "ap-southeast-2", "ap-northeast-1"
    ]),
    "estimatedCost": JSON.stringify({
      "compute": 25,
      "networking": 5,
      "storage": 2,
      "total": 32
    }),
    "owner": "system",
    "public": true
  });
  
  return dao.saveRecord(record);
}, (db) => {
  const dao = new Dao(db);
  const record = dao.findRecordById("awsBlueprints", "ecs_web_app_001");
  return dao.deleteRecord(record);
})