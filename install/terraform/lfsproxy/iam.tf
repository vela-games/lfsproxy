data "aws_caller_identity" "current" {}

resource "aws_iam_policy" "lfsproxy-policy" {
  name = "LFSProxyS3Policy"

  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : [
          "s3:*"
        ],
        "Resource" : [
          "${aws_s3_bucket.lfsproxy.arn}",
          "${aws_s3_bucket.lfsproxy.arn}/*"
        ]
      },
      {
        "Effect" : "Allow",
        "Action" : [
          "sts:AssumeRole"
        ],
        "Resource" : [
          "${aws_iam_role.lfsproxy-role.arn}"
        ]
      },
    ]
  })
}

resource "aws_iam_role" "lfsproxy-role" {
  name = "LFSProxyRole"

  assume_role_policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Principal" : {
          "Federated" : "arn:aws:iam::${data.aws_caller_identity.current.account_id}:oidc-provider/${var.oidc_issuer}"
        },
        "Action" : "sts:AssumeRoleWithWebIdentity",
        "Condition" : {
          "StringEquals" : {
            "${var.oidc_issuer}:aud" : "sts.amazonaws.com"
            "${var.oidc_issuer}:sub" : "system:serviceaccount:lfsproxy:lfsproxy"
          }
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "attach-lfsproxy-policy" {
  role       = aws_iam_role.lfsproxy-role.name
  policy_arn = aws_iam_policy.lfsproxy-policy.arn
}