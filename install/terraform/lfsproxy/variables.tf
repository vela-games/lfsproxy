variable "oidc_issuer" {
  default = ""
}

variable "bucket_name" {
  default = ""
}

variable "s3_vpc_endpoint_id" {
  default = ""
}

variable "s3_replication_iam_role_arn" {
  default = ""
}

variable "replicate_to_bucket_arns" {
  default = []
}