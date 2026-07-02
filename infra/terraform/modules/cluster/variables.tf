variable "name" {
  description = "Cluster name (kubectl context becomes kind-<name>)"
  type        = string
}

variable "node_image" {
  description = "kindest/node image tag — pins the Kubernetes version"
  type        = string
  default     = "kindest/node:v1.31.0"
}

variable "worker_count" {
  description = "Number of worker nodes"
  type        = number
  default     = 2
}

variable "ingress_http_port" {
  description = "Host port mapped to the cluster's ingress HTTP port (80)"
  type        = number
  default     = 80
}

variable "ingress_https_port" {
  description = "Host port mapped to the cluster's ingress HTTPS port (443)"
  type        = number
  default     = 443
}
