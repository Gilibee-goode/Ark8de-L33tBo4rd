# environments/dev — local KIND cluster for the Ark8de stack.
#
# Usage:
#   terraform init
#   terraform apply
#   kubectl cluster-info --context kind-ark8de-dev

terraform {
  required_version = ">= 1.5"

  # Remote state (Terraform Cloud, free tier) — uncomment and set your org
  # after `terraform login`. Until then state is local (dev-only; local
  # .tfstate is gitignored and breaks as soon as CI also runs Terraform).
  #
  # cloud {
  #   organization = "YOUR_TFC_ORG"
  #   workspaces {
  #     name = "ark8de-dev"
  #   }
  # }

  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "~> 0.9"
    }
  }
}

provider "kind" {}

module "cluster" {
  source = "../../modules/cluster"

  name         = var.cluster_name
  node_image   = var.node_image
  worker_count = var.worker_count

  # 80/443 would collide with anything already listening on the laptop, and
  # binding 80 can require elevated privileges — use high ports for dev.
  ingress_http_port  = 8880
  ingress_https_port = 8443
}
