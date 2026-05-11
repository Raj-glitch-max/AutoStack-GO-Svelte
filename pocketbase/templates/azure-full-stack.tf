# Azure Full Stack Blueprint — Container Apps + Azure SQL
# Equivalent to AWS full-stack (ECS + RDS)

variable "app_name" { type = string }
variable "subscription_id" { type = string }
variable "location" { type = string; default = "eastus" }
variable "container_image" { type = string }
variable "db_admin_user" { type = string; default = "adminuser" }
variable "db_admin_password" { type = string; sensitive = true }
variable "db_sku" { type = string; default = "GP_Gen5_2" }
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

resource "azurerm_resource_group" "main" {
  name     = "${var.app_name}-rg"
  location = var.location
  tags     = { ManagedBy = "AutoStack"; UserId = var.user_id; DeploymentId = var.deployment_id }
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

# Azure SQL Server
resource "azurerm_mssql_server" "main" {
  name                         = "${var.app_name}-sqlserver"
  resource_group_name          = azurerm_resource_group.main.name
  location                     = azurerm_resource_group.main.location
  version                      = "12.0"
  administrator_login          = var.db_admin_user
  administrator_login_password = var.db_admin_password
  tags                         = { ManagedBy = "AutoStack" }
}

resource "azurerm_mssql_database" "main" {
  name      = "${var.app_name}-db"
  server_id = azurerm_mssql_server.main.id
  sku_name  = "S0"
}

# Allow Azure services to access SQL
resource "azurerm_mssql_firewall_rule" "azure_services" {
  name             = "AllowAzureServices"
  server_id        = azurerm_mssql_server.main.id
  start_ip_address = "0.0.0.0"
  end_ip_address   = "0.0.0.0"
}

resource "azurerm_container_app" "main" {
  name                         = var.app_name
  container_app_environment_id = azurerm_container_app_environment.main.id
  resource_group_name          = azurerm_resource_group.main.name
  revision_mode                = "Single"
  tags                         = { ManagedBy = "AutoStack"; UserId = var.user_id }

  template {
    min_replicas = 1
    max_replicas = 10

    container {
      name   = var.app_name
      image  = var.container_image
      cpu    = 0.5
      memory = "1Gi"

      env {
        name  = "DATABASE_URL"
        value = "sqlserver://${azurerm_mssql_server.main.fully_qualified_domain_name};database=${azurerm_mssql_database.main.name};user id=${var.db_admin_user};password=${var.db_admin_password}"
      }
    }
  }

  ingress {
    external_enabled = true
    target_port      = 80
    traffic_weight { percentage = 100; latest_revision = true }
  }
}

output "app_url"     { value = "https://${azurerm_container_app.main.ingress[0].fqdn}" }
output "db_server"   { value = azurerm_mssql_server.main.fully_qualified_domain_name }
output "db_name"     { value = azurerm_mssql_database.main.name }
