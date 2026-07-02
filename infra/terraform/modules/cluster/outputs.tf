output "name" {
  description = "Cluster name"
  value       = kind_cluster.this.name
}

output "endpoint" {
  description = "Kubernetes API server endpoint"
  value       = kind_cluster.this.endpoint
}

output "kubeconfig_path" {
  description = "Path to the kubeconfig file written by the provider"
  value       = kind_cluster.this.kubeconfig_path
}

output "kubeconfig" {
  description = "Raw kubeconfig contents (sensitive)"
  value       = kind_cluster.this.kubeconfig
  sensitive   = true
}
