# Hopsworks cluster with RonDB

In this example, we create a Hopsworks cluster with RonDB support. We use an empty `rondb` block which will create a RonDB cluster with 1 management node, 2 data nodes, and 1 mysql node.

## Configure RonDB

You can configure RonDB nodes instead of relying on the default configurations, for instance in the following example, we increased the number of data nodes to 4 and we used an instance type with at least 8 CPUs and 16 GB of memory.

```hcl
data "hopsworksai_instance_type" "smallest_rondb_datanode" {
  cloud_provider = "AWS"
  node_type      = "rondb_data"
  min_memory_gb  = 16
  min_cpus       = 8
}

resource "hopsworksai_cluster" "cluster" {
  # all the other configurations are omitted for clarity 

  rondb {
    data_nodes {
      instance_type = data.hopsworksai_instance_type.smallest_rondb_datanode.id
      disk_size     = 512
      count         = 4
    }
  }
}
```

## How to run the example 
First ensure that your aws credentials are setup correctly by running the following command 
`
```bash
aws configure 
```

Then, run the following commands. Replace the placeholder with your Hopsworks API Key. The cluster will be created in us-east-2 region by default, however, you can configure the region to use by setting the variable region when applying the changes `-var="region=YOUR_REGION"`

```bash
export HOPSWORKSAI_API_KEY=<YOUR_HOPSWORKSAI_API_KEY>
terraform init
terraform apply
```

## Terminate the cluster

You can run `terraform destroy` to delete the cluster and all the other required cloud resources created in this example.