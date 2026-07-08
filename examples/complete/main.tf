terraform {
  required_providers {
    fpcloud = {
      source = "fogpipe/fpcloud"
    }
  }
}

provider "fpcloud" {}

variable "stripe_key" {
  type      = string
  sensitive = true
}

variable "session_secret" {
  type      = string
  sensitive = true
}

# Project
resource "fpcloud_project" "myapp" {
  name = "my-saas-app"
}

# Database
resource "fpcloud_database" "main" {
  project_id = fpcloud_project.myapp.id
  name       = "maindb"
  version    = "17"
  cpu        = "1"
  memory     = "2Gi"
}

# Service account for workload identity
resource "fpcloud_service_account" "app_identity" {
  project_id   = fpcloud_project.myapp.id
  name         = "web-identity"
  display_name = "Web App Workload Identity"
}

# Grant the service account viewer role on the project
resource "fpcloud_iam_binding" "app_read_db" {
  project_id  = fpcloud_project.myapp.id
  role        = "viewer"
  member_type = "serviceAccount"
  member_id   = fpcloud_service_account.app_identity.id
}

# Web application
resource "fpcloud_app" "web" {
  project_id      = fpcloud_project.myapp.id
  name            = "web"
  image           = "ghcr.io/myorg/webapp:latest"
  port            = 3000
  ingress         = "all"
  service_account = fpcloud_service_account.app_identity.email
  min_scale       = 2
  max_scale       = 10

  env = {
    APP_NAME = "My SaaS App"
  }

  secret = {
    STRIPE_SECRET_KEY = var.stripe_key
    SESSION_SECRET    = var.session_secret
  }
}

# Custom domain
resource "fpcloud_domain" "main" {
  app_id = fpcloud_app.web.id
  domain = "app.example.com"
}

# Auto-deploy from GitHub
resource "fpcloud_webhook" "deploy" {
  app_id        = fpcloud_app.web.id
  repo          = "myorg/webapp"
  branch        = "main"
  image_pattern = "ghcr.io/myorg/webapp:{{sha}}"
}

# Outputs
output "app_url" {
  value = fpcloud_app.web.url
}

output "database_host" {
  value = fpcloud_database.main.host
}

output "webhook_url" {
  value = fpcloud_webhook.deploy.webhook_url
}
