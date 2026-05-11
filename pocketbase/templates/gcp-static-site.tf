# GCP Static Site Blueprint — GCS + Cloud CDN
# Equivalent to AWS static-site (S3 + CloudFront)

variable "app_name" { type = string }
variable "gcp_project_id" { type = string }
variable "region" { type = string; default = "us-central1" }
variable "user_id" { type = string }
variable "deployment_id" { type = string }

terraform {
  required_providers {
    google = { source = "hashicorp/google"; version = "~> 5.0" }
  }
}

provider "google" {
  project = var.gcp_project_id
  region  = var.region
}

resource "google_project_service" "storage" { service = "storage.googleapis.com"; disable_on_destroy = false }
resource "google_project_service" "compute" { service = "compute.googleapis.com"; disable_on_destroy = false }

# GCS bucket for static files
resource "google_storage_bucket" "site" {
  name          = "${var.app_name}-site-${var.gcp_project_id}"
  location      = "US"
  force_destroy = true

  website {
    main_page_suffix = "index.html"
    not_found_page   = "404.html"
  }

  uniform_bucket_level_access = true

  labels = {
    managed-by    = "autostack"
    user-id       = var.user_id
    deployment-id = var.deployment_id
  }
}

# Make bucket publicly readable
resource "google_storage_bucket_iam_member" "public" {
  bucket = google_storage_bucket.site.name
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}

# Backend bucket for Cloud CDN
resource "google_compute_backend_bucket" "cdn" {
  name        = "${var.app_name}-cdn-backend"
  bucket_name = google_storage_bucket.site.name
  enable_cdn  = true
  depends_on  = [google_project_service.compute]
}

# URL map
resource "google_compute_url_map" "main" {
  name            = "${var.app_name}-url-map"
  default_service = google_compute_backend_bucket.cdn.id
}

# HTTP proxy
resource "google_compute_target_http_proxy" "main" {
  name    = "${var.app_name}-http-proxy"
  url_map = google_compute_url_map.main.id
}

# Global forwarding rule
resource "google_compute_global_forwarding_rule" "http" {
  name       = "${var.app_name}-http-rule"
  target     = google_compute_target_http_proxy.main.id
  port_range = "80"
}

output "cdn_ip"       { value = google_compute_global_forwarding_rule.http.ip_address }
output "bucket_name"  { value = google_storage_bucket.site.name }
output "bucket_url"   { value = google_storage_bucket.site.url }
