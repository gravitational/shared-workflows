resource "aws_ecr_repository" "approval-service" {
  name                 = "approval-service"

  # TODO: After testing should be set to "IMMUTABLE"
  # The default is "MUTABLE" which allows overwriting images with the same tag
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

output "repository_url" {
  value = aws_ecr_repository.approval-service.repository_url
}

data "aws_route53_zone" "eks-gha-dev" {
   name         = "eks-gha-dev.cluster.teleport.dev"
}

resource "aws_acm_certificate" "approval-service" {
  domain_name       = "approval-service.eks-gha-dev.cluster.teleport.dev"
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}
