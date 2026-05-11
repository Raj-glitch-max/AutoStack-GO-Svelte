# Azure Static Site Blueprint — Blob Storage + Azure CDN
# Equivalent to AWS static-site (S3 + CloudFront)

variable "app_name" { type = string }
variable "subscription_id" { type = string }
variable "location" { type = string; default = "eastus" }
variable "user_id" { type = string }
variable "deployment_id" { type = string }

terraform {
  required_providers {
    azurerm = { source = "hashicorp/azurerm"; version = "~> 3.0" }
    random  = { source = "hashicorp/random"; version = "~> 3.0" }
  }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

resource "random_string" "suffix" {
  length  = 6
  special = false
  upper   = false
}

resource "azurerm_resource_group" "main" {
  name     = "${var.app_name}-rg"
  location = var.location
  tags     = { ManagedBy = "AutoStack"; UserId = var.user_id; DeploymentId = var.deployment_id }
}

resource "azurerm_storage_account" "main" {
  name                     = "${replace(var.app_name, "-", "")}${random_string.suffix.result}"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  account_kind             = "StorageV2"

  static_website {
    index_document     = "index.html"
    error_404_document = "404.html"
  }

  tags = { ManagedBy = "AutoStack" }
}

resource "azurerm_cdn_profile" "main" {
  name                = "${var.app_name}-cdn"
  location            = "global"
  resource_group_name = azurerm_resource_group.main.name
  sku                 = "Standard_Microsoft"
}

resource "azurerm_cdn_endpoint" "main" {
  name                = "${var.app_name}-endpoint"
  profile_name        = azurerm_cdn_profile.main.name
  location            = "global"
  resource_group_name = azurerm_resource_group.main.name

  origin {
    name      = "storage"
    host_name = azurerm_storage_account.main.primary_web_host
  }

  origin_host_header = azurerm_storage_account.main.primary_web_host
}

output "cdn_url"          { value = "https://${azurerm_cdn_endpoint.main.host_name}" }
output "storage_account"  { value = azurerm_storage_account.main.name }
output "static_web_url"   { value = azurerm_storage_account.main.primary_web_endpoint }
