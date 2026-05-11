# GCP Full Stack Blueprint — Cloud Run + Cloud SQL
# Equivalent to AWS full-stack (ECS + RDS)

variable "app_name" { type = string }
variable "gcp_project_id" { type = string }
variable "region" { type = string; default = "us-central1" }
variable "container_image" { type = string }
variable "db_tier" { type = string; default = "db-f1-micro" }
variable "db_name" { type = string; default = "appdb" }
variable "db_user" { type = string; default = "appuser" }
variable "db_password" { type = string; sensitive = true }
variable "user_id" { type = string }
variable "deployment_id" { type = string }

terraform {
  required_providers {
    google = { source = "hashicorp/google"; version = "~> 5.0" }
    random = { source = "hashicorp/random"; version = "~> 3.0" }
  }
}

provider "google" {
  project = var.gcp_project_id
  region  = var.region
}

resource "google_project_service" "run"  { service = "run.googleapis.com";  disable_on_destroy = false }
resource "google_project_service" "sql"  { service = "sqladmin.googleapis.com"; disable_on_destroy = false }
resource "google_project_service" "vpc"  { service = "servicenetworking.googleapis.com"; disable_on_destroy = false }

# VPC for private Cloud SQL connectivity
resource "google_compute_network" "main" {
  name                    = "${var.app_name}-vpc"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "main" {
  name          = "${var.app_name}-subnet"
  ip_cidr_range = "10.0.0.0/24"
  region        = var.region
  network       = google_compute_network.main.id
}

# Private IP for Cloud SQL
resource "google_compute_global_address" "sql_private" {
  name          = "${var.app_name}-sql-ip"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.main.id
}

resource "google_service_networking_connection" "sql" {
  network                 = google_compute_network.main.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.sql_private.name]
  depends_on              = [google_project_service.vpc]
}

# Cloud SQL (PostgreSQL)
resource "google_sql_database_instance" "main" {
  name             = "${var.app_name}-db"
  database_version = "POSTGRES_15"
  region           = var.region

  settings {
    tier = var.db_tier

    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.main.id
    }

    backup_configuration {
      enabled = true
    }
  }

  deletion_protection = false
  depends_on          = [google_service_networking_connection.sql]
}

resource "google_sql_database" "app" {
  name     = var.db_name
  instance = google_sql_database_instance.main.name
}

resource "google_sql_user" "app" {
  name     = var.db_user
  instance = google_sql_database_instance.main.name
  password = var.db_password
}

# Service account for Cloud Run
resource "google_service_account" "run" {
  account_id   = "${var.app_name}-run-sa"
  display_name = "${var.app_name} Cloud Run SA"
}

resource "google_project_iam_member" "sql_client" {
  project = var.gcp_project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.run.email}"
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

    vpc_access {
      network_interfaces {
        network    = google_compute_network.main.id
        subnetwork = google_compute_subnetwork.main.id
      }
    }

    containers {
      image = var.container_image

      env {
        name  = "DATABASE_URL"
        value = "postgresql://${var.db_user}:${var.db_password}@${google_sql_database_instance.main.private_ip_address}:5432/${var.db_name}"
      }

      resources {
        limits = { cpu = "1"; memory = "512Mi" }
      }
    }
  }

  depends_on = [google_project_service.run]
}

resource "google_cloud_run_v2_service_iam_member" "public" {
  project  = var.gcp_project_id
  location = var.region
  name     = google_cloud_run_v2_service.main.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

output "service_url"    { value = google_cloud_run_v2_service.main.uri }
output "db_private_ip"  { value = google_sql_database_instance.main.private_ip_address }
output "service_name"   { value = google_cloud_run_v2_service.main.name }
