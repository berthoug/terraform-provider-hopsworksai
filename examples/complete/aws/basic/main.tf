provider "aws" {
  region  = var.region
  profile = var.profile
}

provider "hopsworksai" {
  api_key = "VxTYBPrW6H7nZghSMBW9x6FMRcV9eH4I6ujIimxb"
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

data "aws_ami" "tester" {
  most_recent = true

  filter {
    name   = "name"
    values = ["DEV-hopsworks-cluster-ubuntu-3.1.0-SNAPSHOT_worker*"]
  }


  owners = ["244384877559"]
}


resource "aws_instance" "testser" {
  ami           = data.aws_ami.tester.id
  instance_type = "t3.micro"

  tags = {
    Owner = "gautier"
  }
  subnet_id                   = hopsworksai_cluster.cluster.aws_attributes[0].network[0].subnet_id
  key_name                    = module.aws.ssh_key_pair_name
  associate_public_ip_address = true
  vpc_security_group_ids      = [hopsworksai_cluster.cluster.aws_attributes[0].network[0].security_group_id]

  user_data_replace_on_change = true
  user_data                   = templatefile("config/templates/tester_init.sh", {
    head_ip = hopsworksai_cluster.cluster.head[0].private_ip,
    region = var.region,
    environment = var.environment
  })
}