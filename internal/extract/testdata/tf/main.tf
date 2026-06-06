variable "region" { default = "us-east-1" }

data "aws_ami" "ubuntu" { most_recent = true }

resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }

resource "aws_instance" "web" {
  ami        = data.aws_ami.ubuntu.id
  subnet_id  = aws_vpc.main.id
  region     = var.region
  depends_on = [aws_vpc.main]
}
