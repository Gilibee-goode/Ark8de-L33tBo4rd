# modules/cluster — provider-agnostic cluster interface.
#
# Dev implementation: a local KIND (Kubernetes IN Docker) cluster via the
# tehcyx/kind provider. For prod, swap this module's internals for a cloud
# provider (Hetzner/DO/AWS) — the module's variables/outputs stay the same.

terraform {
  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "~> 0.9"
    }
  }
}

resource "kind_cluster" "this" {
  name           = var.name
  node_image     = var.node_image
  wait_for_ready = true

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    # Control-plane node doubles as the ingress node: host ports 80/443 are
    # mapped into it so ingress-nginx (NodePort/hostPort) is reachable from
    # the laptop at http://localhost.
    node {
      role = "control-plane"

      kubeadm_config_patches = [
        <<-EOT
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
        EOT
      ]

      extra_port_mappings {
        container_port = 80
        host_port      = var.ingress_http_port
      }
      extra_port_mappings {
        container_port = 443
        host_port      = var.ingress_https_port
      }
    }

    # Worker nodes run the application pods.
    dynamic "node" {
      for_each = range(var.worker_count)
      content {
        role = "worker"
      }
    }
  }
}
