# Azure Container Apps Blueprint
# Equivalent to AWS ecs-web-app

variable "app_name" { type = string }
variable "subscription_id" { type = string }
variable "resource_group" { type = string; default = "" }
variable "location" { type = string; default = "eastus" }
variable "container_image" { type = string }
variable "cpu" { type = number; default = 0.5 }
variable "memory" { type = string; default = "1Gi" }
variable "min_replicas" { type = number; default = 0 }
variable "max_replicas" { type = number; default = 10 }
variable "user_id" { type = string }
variable "deployment_id" { type = string }

terraform {
  required_providers {
    azurerm = { source = "hashicorp/azurerm"; version = "~> 3.0" }
  }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

locals {
  rg_name = var.resource_group != "" ? var.resource_group : "${var.app_name}-rg"
}

resource "azurerm_resource_group" "main" {
  name     = local.rg_name
  location = var.location
  tags = {
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    DeploymentId = var.deployment_id
  }
}

resource "azurerm_log_analytics_workspace" "main" {
  name                = "${var.app_name}-logs"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  sku                 = "PerGB2018"
  retention_in_days   = 30
}

resource "azurerm_container_app_environment" "main" {
  name                       = "${var.app_name}-env"
  location                   = azurerm_resource_group.main.location
  resource_group_name        = azurerm_resource_group.main.name
  log_analytics_workspace_id = azurerm_log_analytics_workspace.main.id
}

resource "azurerm_container_app" "main" {
  name                         = var.app_name
  container_app_environment_id = azurerm_container_app_environment.main.id
  resource_group_name          = azurerm_resource_group.main.name
  revision_mode                = "Single"

  tags = {
    ManagedBy    = "AutoStack"
    UserId       = var.user_id
    DeploymentId = var.deployment_id
  }

  template {
    min_replicas = var.min_replicas
    max_replicas = var.max_replicas

    container {
      name   = var.app_name
      image  = var.container_image
      cpu    = var.cpu
      memory = var.memory
    }
  }

  ingress {
    external_enabled = true
    target_port      = 80
    traffic_weight {
      percentage      = 100
      latest_revision = true
    }
  }
}

output "app_url"            { value = "https://${azurerm_container_app.main.ingress[0].fqdn}" }
output "app_name"           { value = azurerm_container_app.main.name }
output "resource_group"     { value = azurerm_resource_group.main.name }
