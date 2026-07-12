# Azure Container Apps Deployment for Helix RPC
# Deploys the Helix container to Azure Container Apps.

provider "azurerm" {
  features {}
}

resource "azurerm_resource_group" "helix_rg" {
  name     = "helix-resources"
  location = "East US"
}

resource "azurerm_container_app_environment" "helix_env" {
  name                = "helix-environment"
  location            = azurerm_resource_group.helix_rg.location
  resource_group_name = azurerm_resource_group.helix_rg.name
}

resource "azurerm_container_app" "helix_app" {
  name                         = "helix-app"
  container_app_environment_id = azurerm_container_app_environment.helix_env.id
  resource_group_name          = azurerm_resource_group.helix_rg.name
  revision_mode                = "Single"

  template {
    container {
      name   = "helix-container"
      image  = "myacr.azurecr.io/helix-service:latest"
      cpu    = "1.0"
      memory = "2.0Gi"
    }
  }

  ingress {
    allow_insecure_connections = false
    external_enabled           = true
    target_port                = 8080
    transport                  = "auto" # automatically sniffs protocol
  }
}
