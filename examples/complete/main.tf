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
  cpu        = "1"
  memory     = "2Gi"
}

# Web application
resource "fpcloud_app" "web" {
  project_id      = fpcloud_project.myapp.id
  name            = "web"
  image           = "ghcr.io/myorg/webapp:latest"
  port            = 3000
  ingress         = "all"
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

# The app runs as its own service account, which holds nothing until granted.
resource "fpcloud_iam_binding" "web_identity" {
  project_id  = fpcloud_project.myapp.id
  role        = "viewer"
  member_type = "serviceAccount"
  member_id   = fpcloud_app.web.service_account
}

# A resource handed to the app's identity: a database or a bucket as its user,
# a secret as its reader. Mounting the database's owner secret into the app
# needs this grant, and writes it when the applier may make it.
resource "fpcloud_iam_binding" "web_uses_db" {
  project_id  = fpcloud_project.myapp.id
  resource    = "database/${fpcloud_database.main.id}"
  role        = "user"
  member_type = "serviceAccount"
  member_id   = fpcloud_app.web.service_account
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
