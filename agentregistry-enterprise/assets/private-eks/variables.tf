variable "cluster_name" {
  type    = string
  default = "are-private-demo"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "k8s_version" {
  type    = string
  default = "1.33"
}

variable "vpc_cidr" {
  type    = string
  default = "10.0.0.0/16"
}

variable "node_instance_type" {
  type    = string
  default = "t3.xlarge"
}

variable "desired_capacity" {
  type    = number
  default = 2
}

variable "min_capacity" {
  type    = number
  default = 1
}

variable "max_capacity" {
  type    = number
  default = 4
}

variable "endpoint_public_access" {
  type        = bool
  default     = true
  description = "Enable public API server endpoint for initial setup. Set to false after configuring VPN/bastion for a fully private cluster."
}

variable "public_access_cidrs" {
  type        = list(string)
  default     = ["0.0.0.0/0"]
  description = "CIDR blocks that can access the public API server endpoint. Restrict this to your IP for security."
}
