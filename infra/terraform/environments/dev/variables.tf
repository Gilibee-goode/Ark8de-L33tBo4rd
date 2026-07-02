variable "cluster_name" {
  description = "Name of the dev KIND cluster"
  type        = string
  default     = "ark8de-dev"
}

variable "node_image" {
  description = "kindest/node image (Kubernetes version)"
  type        = string
  default     = "kindest/node:v1.31.0"
}

variable "worker_count" {
  description = "Number of worker nodes"
  type        = number
  default     = 2
}
