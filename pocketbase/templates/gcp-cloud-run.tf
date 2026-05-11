# GCP Cloud Run Web App Blueprint
# Equivalent to AWS ecs-web-app — stateless containerized web application

variable "app_name" {
  description = "Application name"
  type        = string
}

variable "gcp_project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "container_image" {
  description = "Docker container image (must be in GCR or Artifact Registry)"
  type        = string
}

variable "user_id" {
  description = "AutoStack user ID for tagging"
  type        = string
}

variable "deployment_id" {
  description = "AutoStack deployment ID for tagging"
  type        = string
}

variable "min_instances" {
  description = "Minimum number of Cloud Run instances"
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum number of Cloud Run instances"
  type        = number
  default     = 10
}

variable "cpu" {
  description = "CPU allocation per instance (e.g. '1', '2')"
  type        = string
  default     = "1"
}

variable "memory" {
  description = "Memory allocation per instance (e.g. '512Mi', '1Gi')"
  type        = string
  default     = "512Mi"
}

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.gcp_project_id
  region  = var.region
}

# Enable required APIs
resource "google_project_service" "run" {
  service            = "run.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "iam" {
  service            = "iam.googleapis.com"
  disable_on_destroy = false
}

# Service account for Cloud Run
resource "google_service_account" "run" {
  account_id   = "${var.app_name}-run-sa"
  display_name = "${var.app_name} Cloud Run Service Account"
}

# Cloud Run service
resource "google_cloud_run_v2_service" "main" {
  name     = var.app_name
  location = var.region

  labels = {
    managed-by    = "autostack"
    user-id       = var.user_id
    deployment-id = var.deployment_id
  }

  template {
    service_account = google_service_account.run.email

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      image = var.container_image

      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
      }

      ports {
        container_port = 8080
      }
    }
  }

  depends_on = [google_project_service.run]
}

# Allow unauthenticated access (public web app)
resource "google_cloud_run_v2_service_iam_member" "public" {
  project  = var.gcp_project_id
  location = var.region
  name     = google_cloud_run_v2_service.main.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Outputs
output "service_url" {
  description = "Cloud Run service URL"
  value       = google_cloud_run_v2_service.main.uri
}

output "service_name" {
  description = "Cloud Run service name"
  value       = google_cloud_run_v2_service.main.name
}

output "region" {
  description = "Deployment region"
  value       = var.region
}
