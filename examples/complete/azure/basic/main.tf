provider "azurerm" {
  features {}
  skip_provider_registration = true
}

provider "hopsworksai" {
}

# Create required azure resources, an ssh key, a storage account, and an user assigned managed identity with the required hopsworks permissions
module "azure" {
  source         = "logicalclocks/helpers/hopsworksai//modules/azure"
  resource_group = var.resource_group
}

# Create a simple cluster with two workers with two different configuration

data "hopsworksai_instance_type" "small_worker" {
  cloud_provider = "AZURE"
  node_type      = "worker"
  min_memory_gb  = 16
  min_cpus       = 4
}

data "hopsworksai_instance_type" "large_worker" {
  cloud_provider = "AZURE"
  node_type      = "worker"
  min_memory_gb  = 32
  min_cpus       = 4
}

resource "hopsworksai_cluster" "cluster" {
  name    = "tf-hopsworks-cluster"
  ssh_key = module.azure.ssh_key_pair_name

  head {
  }

  workers {
    instance_type = data.hopsworksai_instance_type.small_worker.id
    disk_size     = 256
    count         = 1
  }

  workers {
    instance_type = data.hopsworksai_instance_type.large_worker.id
    disk_size     = 512
    count         = 1
  }

  azure_attributes {
    location                       = module.azure.location
    resource_group                 = module.azure.resource_group
    storage_account                = module.azure.storage_account_name
    user_assigned_managed_identity = module.azure.user_assigned_identity_name
  }

  open_ports {
    ssh = true
  }
}
