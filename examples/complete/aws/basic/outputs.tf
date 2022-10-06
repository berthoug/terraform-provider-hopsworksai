output "hopsworks_cluster_url" {
  value = hopsworksai_cluster.cluster.url
}

output "head_node_ip" {
  value = hopsworksai_cluster.cluster.head[0].private_ip
}


output "tester_public_ip" {
  value = aws_instance.testser.public_ip
}
