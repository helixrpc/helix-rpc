# Google Cloud Run Deployment for Helix RPC
# Deploys the Helix container to GCP Cloud Run with HTTP/2 enabled.

provider "google" {
  project = "my-gcp-project"
  region  = "us-central1"
}

resource "google_cloud_run_service" "helix_service" {
  name     = "helix-service"
  location = "us-central1"

  template {
    spec {
      containers {
        image = "gcr.io/my-gcp-project/helix-service:latest"
        ports {
          container_port = 8080
          # Enable HTTP/2 for gRPC multiplexing support
          name = "h2c"
        }
        resources {
          limits = {
            cpu    = "2000m" # 2 vCPUs
            memory = "2Gi"
          }
        }
      }
    }
  }

  traffic {
    percent         = 100
    latest_revision = true
  }
}

resource "google_cloud_run_service_iam_member" "public_access" {
  service  = google_cloud_run_service.helix_service.name
  location = google_cloud_run_service.helix_service.location
  role     = "roles/run.invoker"
  member   = "allUsers"
}
