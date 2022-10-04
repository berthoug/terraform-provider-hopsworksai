provider "aws" {
  region  = var.region
  profile = var.profile
}

provider "hopsworksai" {
  api_key     = "lYC0V9BTp48YfDlrJtqvB6q53S8cS1QB1BHuXBoq"
  api_gateway = "https://qaicacl203.execute-api.us-east-2.amazonaws.com/gautier"
}

# Create required aws resources, an ssh key, an s3 bucket, and an instance profile with the required hopsworks permissions
module "aws" {
  source  = "logicalclocks/helpers/hopsworksai//modules/aws"
  region  = var.region
  version = "2.0.0"
}

# Create a simple cluster with two workers with two different configuration

data "hopsworksai_instance_type" "head" {
  cloud_provider = "AWS"
  node_type      = "head"
  region         = var.region
}

data "hopsworksai_instance_type" "rondb_mgm" {
  cloud_provider = "AWS"
  node_type      = "rondb_management"
  region         = var.region
}

data "hopsworksai_instance_type" "rondb_data" {
  cloud_provider = "AWS"
  node_type      = "rondb_data"
  region         = var.region
  min_memory_gb  = 32
}

data "hopsworksai_instance_type" "rondb_mysql" {
  cloud_provider = "AWS"
  node_type      = "rondb_mysql"
  region         = var.region
}

data "hopsworksai_instance_type" "small_worker" {
  cloud_provider = "AWS"
  node_type      = "worker"
  region         = var.region
  min_memory_gb  = 16
  min_cpus       = 4
}

data "hopsworksai_instance_type" "large_worker" {
  cloud_provider = "AWS"
  node_type      = "worker"
  region         = var.region
  min_memory_gb  = 32
  min_cpus       = 4
}

resource "hopsworksai_cluster" "cluster" {
  name          = "test"
  ssh_key       = module.aws.ssh_key_pair_name
  managed_users = false
  version       = "3.1.0-SNAPSHOT"

  head {
    instance_type = data.hopsworksai_instance_type.head.id
  }

  workers {
    instance_type = data.hopsworksai_instance_type.small_worker.id
    disk_size     = 256
    count         = 0
  }

  workers {
    instance_type = data.hopsworksai_instance_type.large_worker.id
    disk_size     = 512
    count         = 1
  }

  aws_attributes {
    region = var.region
    bucket {
      name = module.aws.bucket_name
    }
    instance_profile_arn = module.aws.instance_profile_arn
  }

  rondb {
    single_node {
      instance_type = data.hopsworksai_instance_type.rondb_data.id
    }
  }

  open_ports {
    ssh = true
  }

  init_script = templatefile("config/templates/hopsworks_init.sh", {})
}

data "aws_ami" "ubuntu" {
  most_recent = true

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["099720109477"] # Canonical
}


resource "aws_instance" "testser" {
  ami           = data.aws_ami.ubuntu.id
  instance_type = "t3.micro"

  tags = {
    Owner = "gautier"
  }
  subnet_id                   = hopsworksai_cluster.cluster.aws_attributes[0].network[0].subnet_id
  key_name                    = module.aws.ssh_key_pair_name
  associate_public_ip_address = true
  vpc_security_group_ids      = [hopsworksai_cluster.cluster.aws_attributes[0].network[0].security_group_id]

  user_data_replace_on_change = true
  user_data                   = <<-EOF
    #!/bin/bash
    apt update
    apt install -y mysql-client-core-8.0 python3-pip
    !pip install -U hopsworks --quiet
  EOF
}