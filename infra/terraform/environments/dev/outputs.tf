output "cluster_name" {
  description = "KIND cluster name (kubectl context: kind-<name>)"
  value       = module.cluster.name
}

output "endpoint" {
  description = "Kubernetes API endpoint"
  value       = module.cluster.endpoint
}

output "kubeconfig_path" {
  description = "Path to the generated kubeconfig"
  value       = module.cluster.kubeconfig_path
}
