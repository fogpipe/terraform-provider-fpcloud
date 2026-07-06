terraform {
  required_providers {
    fpcloud = {
      source = "fogpipe/fpcloud"
    }
  }
}

provider "fpcloud" {
  # api_key = "fp-..."           # Or set FPCLOUD_API_KEY env var
  # api_url = "https://api.cloud.fogpipe.com"  # Or set FPCLOUD_API_URL env var
}
