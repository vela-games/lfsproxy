resource "aws_s3_bucket" "lfsproxy" {
  bucket = var.bucket_name

  tags = {
    Name = "LFS Proxy Cache"
  }
}

resource "aws_s3_bucket_ownership_controls" "disable_s3_acl" {
  bucket = aws_s3_bucket.lfsproxy.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_versioning" "s3_versioning" {
  bucket = aws_s3_bucket.lfsproxy.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_policy" "allow_access_from_within_vpce" {
  count = var.s3_vpc_endpoint_id != "" ? 1 : 0

  bucket = aws_s3_bucket.lfsproxy.id
  policy = data.aws_iam_policy_document.allow_from_vpce.json
}

data "aws_iam_policy_document" "allow_from_vpce" {
  statement {
    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test = "StringEquals"
      variable = "aws:sourceVpce"
      values = [var.s3_vpc_endpoint_id]
    }

    actions = [
      "s3:*",
    ]

    resources = [
      aws_s3_bucket.lfsproxy.arn,
      "${aws_s3_bucket.lfsproxy.arn}/*",
    ]
  }
}

resource "aws_s3_bucket_replication_configuration" "replication" {
  count = length(var.replicate_to_bucket_arns) > 0 ? 1 : 0
  
  
  depends_on = [aws_s3_bucket_versioning.s3_versioning]

  role   = var.s3_replication_iam_role_arn
  bucket = aws_s3_bucket.lfsproxy.id

  rule {
    id = "replicate"
    status = "Enabled"

    dynamic "destination" {
      for_each = toset(var.replicate_to_bucket_arns)

      content {
        bucket = destination.key
        storage_class = "STANDARD"
      }
    }
  }
}