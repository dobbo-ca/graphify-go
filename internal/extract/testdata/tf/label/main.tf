module "this" {
  source      = "cloudposse/label/null"
  version     = "0.25.0"
  namespace   = "eg"
  environment = "ue1"
  stage       = "prod"
  name        = "app"
  attributes  = ["public"]
  delimiter   = "-"
}

resource "aws_s3_bucket" "default" {
  bucket = module.this.id
  tags   = module.this.tags
}
