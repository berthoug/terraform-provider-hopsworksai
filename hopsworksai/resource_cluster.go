package hopsworksai

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/logicalclocks/terraform-provider-hopsworksai/hopsworksai/internal/api"
	"github.com/logicalclocks/terraform-provider-hopsworksai/hopsworksai/internal/helpers"
	"github.com/logicalclocks/terraform-provider-hopsworksai/hopsworksai/internal/structure"
)

const (
	awsDefaultInstanceType   = "m5.2xlarge"
	azureDefaultInstanceType = "Standard_D8_v3"
)

func instanceProfileRegex() *regexp.Regexp {
	return regexp.MustCompile(`^arn:aws:iam::([0-9]*):instance-profile/(.*)$`)
}

func defaultRonDBConfiguration(cloud api.CloudProvider) api.RonDBConfiguration {
	ronDB := api.RonDBConfiguration{
		Configuration: api.RonDBBaseConfiguration{
			NdbdDefault: api.RonDBNdbdDefaultConfiguration{
				ReplicationFactor: 2,
			},
			General: api.RonDBGeneralConfiguration{
				Benchmark: api.RonDBBenchmarkConfiguration{
					GrantUserPrivileges: false,
				},
			},
		},
		ManagementNodes: api.WorkerConfiguration{
			NodeConfiguration: api.NodeConfiguration{
				DiskSize: 30,
			},
			Count: 1,
		},
		DataNodes: api.WorkerConfiguration{
			NodeConfiguration: api.NodeConfiguration{
				DiskSize: 512,
			},
			Count: 2,
		},
		MYSQLNodes: api.WorkerConfiguration{
			NodeConfiguration: api.NodeConfiguration{
				DiskSize: 128,
			},
			Count: 1,
		},
		APINodes: api.WorkerConfiguration{
			NodeConfiguration: api.NodeConfiguration{
				DiskSize: 30,
			},
			Count: 0,
		},
	}
	switch cloud {
	case api.AWS:
		ronDB.ManagementNodes.InstanceType = "t3a.medium"
		ronDB.DataNodes.InstanceType = "t3a.xlarge"
		ronDB.MYSQLNodes.InstanceType = "t3a.medium"
		ronDB.APINodes.InstanceType = "t3a.medium"
	case api.AZURE:
		ronDB.ManagementNodes.InstanceType = "Standard_D2s_v4"
		ronDB.DataNodes.InstanceType = "Standard_D4s_v4"
		ronDB.MYSQLNodes.InstanceType = "Standard_D2s_v4"
		ronDB.APINodes.InstanceType = "Standard_D2s_v4"
	}
	return ronDB
}

func defaultAutoscaleConfiguration() api.AutoscaleConfigurationBase {
	return api.AutoscaleConfigurationBase{
		DiskSize:          512,
		MinWorkers:        0,
		MaxWorkers:        10,
		StandbyWorkers:    0.5,
		DownscaleWaitTime: 300,
	}
}

func defaultSpotConfig() api.SpotConfiguration {
	return api.SpotConfiguration{
		MaxPrice:         100,
		FallBackOnDemand: true,
	}
}

func spotSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"max_price_percent": {
			Description:  "The maximum spot instance price in percentage of the on-demand price.",
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      100,
			ValidateFunc: validation.IntBetween(1, 200),
		},
		"fall_back_on_demand": {
			Description: "Fall back to on demand instance if unable to allocate a spot instance",
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     true,
		},
	}
}

func clusterSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"cluster_id": {
			Description: "The Id of the cluster.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"name": {
			Description: "The name of the cluster, must be unique.",
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
		},
		"version": {
			Description: "The version of the cluster.",
			Type:        schema.TypeString,
			Optional:    true,
			ForceNew:    true,
			Default:     "2.2.0",
		},
		"head": {
			Description: "The configurations of the head node of the cluster.",
			Type:        schema.TypeList,
			Required:    true,
			ForceNew:    true,
			MaxItems:    1,
			MinItems:    1,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"instance_type": {
						Description: fmt.Sprintf("The instance type of the head node. Defaults to %s for AWS and %s for Azure.", awsDefaultInstanceType, azureDefaultInstanceType),
						Type:        schema.TypeString,
						Optional:    true,
						Computed:    true,
						ForceNew:    true,
					},
					"disk_size": {
						Description:  "The disk size of the head node in units of GB.",
						Type:         schema.TypeInt,
						Optional:     true,
						ForceNew:     true,
						Default:      512,
						ValidateFunc: validation.IntAtLeast(256),
					},
				},
			},
		},
		"workers": {
			Description:   "The configurations of worker nodes. You can add as many as you want of this block to create workers with different configurations.",
			Type:          schema.TypeSet,
			Optional:      true,
			Set:           helpers.WorkerSetHash,
			ConflictsWith: []string{"autoscale"},
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"instance_type": {
						Description: "The instance type of the worker nodes.",
						Type:        schema.TypeString,
						Required:    true,
					},
					"disk_size": {
						Description:  "The disk size of worker nodes in units of GB",
						Type:         schema.TypeInt,
						Optional:     true,
						Default:      512,
						ValidateFunc: validation.IntAtLeast(0),
					},
					"count": {
						Description:  "The number of worker nodes.",
						Type:         schema.TypeInt,
						Optional:     true,
						Default:      1,
						ValidateFunc: validation.IntAtLeast(0),
					},
					"spot_config": {
						Description: "The configuration to use spot instances",
						Type:        schema.TypeList,
						Optional:    true,
						MaxItems:    1,
						MinItems:    1,
						Elem: &schema.Resource{
							Schema: spotSchema(),
						},
					},
				},
			},
		},
		"ssh_key": {
			Description: "The ssh key name that will be attached to this cluster.",
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
		},
		"backup_retention_period": {
			Description: "The validity of cluster backups in days. If set to 0 cluster backups are disabled.",
			Type:        schema.TypeInt,
			Optional:    true,
			ForceNew:    true,
			Default:     0,
			ValidateFunc: func(val interface{}, key string) (warnings []string, errors []error) {
				v := val.(int)
				if v != 0 && v < 7 {
					errors = append(errors, fmt.Errorf("%q must be either 0 (disabled) or at least 7 days, got: %d", key, v))
				}
				return
			},
		},
		"url": {
			Description: "The url generated to access the cluster.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"tags": {
			Description: "The list of custom tags to be attached to the cluster.",
			Type:        schema.TypeMap,
			Optional:    true,
			ForceNew:    true,
			Elem: &schema.Schema{
				Type: schema.TypeString,
			},
		},
		"issue_lets_encrypt_certificate": {
			Description: "Enable or disable issuing let's encrypt certificates. This can be used to disable issuing certificates if port 80 can not be open.",
			Type:        schema.TypeBool,
			Optional:    true,
			ForceNew:    true,
			Default:     true,
		},
		"attach_public_ip": {
			Description: "Attach or do not attach a public ip to the cluster. This can be useful if you intend to create a cluster in a private network.",
			Type:        schema.TypeBool,
			Optional:    true,
			ForceNew:    true,
			Default:     true,
		},
		"managed_users": {
			Description: "Enable or disable Hopsworks.ai to manage your users.",
			Type:        schema.TypeBool,
			Optional:    true,
			ForceNew:    true,
			Default:     true,
		},
		"state": {
			Description: "The current state of the cluster.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"activation_state": {
			Description: "The current activation state of the cluster.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"creation_date": {
			Description: "The creation date of the cluster. The date is represented in RFC3339 format.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"start_date": {
			Description: "The starting date of the cluster. The date is represented in RFC3339 format.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"update_state": {
			Description:  "The action you can use to start or stop the cluster.",
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "none",
			ValidateFunc: validation.StringInSlice([]string{"none", "start", "stop"}, false),
		},
		"aws_attributes": {
			Description:  "The configurations required to run the cluster on Amazon AWS.",
			Type:         schema.TypeList,
			Optional:     true,
			ForceNew:     true,
			MaxItems:     1,
			Elem:         awsAttributesSchema(),
			ExactlyOneOf: []string{"aws_attributes", "azure_attributes"},
		},
		"azure_attributes": {
			Description:  "The configurations required to run the cluster on Microsoft Azure.",
			Type:         schema.TypeList,
			Optional:     true,
			ForceNew:     true,
			MaxItems:     1,
			Elem:         azureAttributesSchema(),
			ExactlyOneOf: []string{"aws_attributes", "azure_attributes"},
		},
		"open_ports": {
			Description: "Open the required ports to communicate with one of the Hopsworks services.",
			Type:        schema.TypeList,
			Optional:    true,
			Computed:    true,
			MaxItems:    1,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"feature_store": {
						Description: "Open the required ports to access the feature store from outside Hopsworks.",
						Type:        schema.TypeBool,
						Optional:    true,
						Default:     false,
					},
					"online_feature_store": {
						Description: "Open the required ports to access the online feature store from outside Hopsworks.",
						Type:        schema.TypeBool,
						Optional:    true,
						Default:     false,
					},
					"kafka": {
						Description: "Open the required ports to access kafka from outside Hopsworks.",
						Type:        schema.TypeBool,
						Optional:    true,
						Default:     false,
					},
					"ssh": {
						Description: "Open the ssh port (22) to allow ssh access to your cluster.",
						Type:        schema.TypeBool,
						Optional:    true,
						Default:     false,
					},
				},
			},
		},
		"rondb": {
			Description: "Setup a cluster with managed RonDB.",
			Type:        schema.TypeList,
			Optional:    true,
			ForceNew:    true,
			MaxItems:    1,
			Elem:        ronDBSchema(),
		},
		"autoscale": {
			Description:   "Setup auto scaling.",
			Type:          schema.TypeList,
			Optional:      true,
			MaxItems:      1,
			ConflictsWith: []string{"workers"},
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"non_gpu_workers": {
						Description: "Setup auto scaling for non gpu nodes.",
						Type:        schema.TypeList,
						Required:    true,
						MaxItems:    1,
						Elem:        autoscaleSchema(),
					},
					"gpu_workers": {
						Description: "Setup auto scaling for gpu nodes.",
						Type:        schema.TypeList,
						Optional:    true,
						MaxItems:    1,
						Elem:        autoscaleSchema(),
					},
				},
			},
		},
		"init_script": {
			Description: "A bash script that will run on all nodes during their initialization (must start with #!/usr/bin/env bash)",
			Type:        schema.TypeString,
			Optional:    true,
			ForceNew:    true,
		},
		"os": {
			Description:  "The operating system to use for the instances. Supported systems are ubuntu in all regions and centos in some specific regions",
			Type:         schema.TypeString,
			Optional:     true,
			ForceNew:     true,
			Default:      "ubuntu",
			ValidateFunc: validation.StringInSlice([]string{"ubuntu", "centos"}, false),
		},
	}
}

func ronDBSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"configuration": {
				Description: "The configuration of RonDB.",
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"ndbd_default": {
							Description: "The configuration of RonDB data nodes.",
							Type:        schema.TypeList,
							Optional:    true,
							Computed:    true,
							ForceNew:    true,
							MaxItems:    1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"replication_factor": {
										Description: "The number of replicas created by RonDB for high availability.",
										Type:        schema.TypeInt,
										Optional:    true,
										ForceNew:    true,
										Default:     defaultRonDBConfiguration("").Configuration.NdbdDefault.ReplicationFactor,
									},
								},
							},
						},
						"general": {
							Description: "The general configurations of RonDB.",
							Type:        schema.TypeList,
							Optional:    true,
							Computed:    true,
							ForceNew:    true,
							MaxItems:    1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"benchmark": {
										Description: "The configurations required to benchmark RonDB.",
										Type:        schema.TypeList,
										Optional:    true,
										Computed:    true,
										ForceNew:    true,
										MaxItems:    1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"grant_user_privileges": {
													Description: "This allow API nodes to have user privileges access to RonDB. This is needed mainly for benchmarking and for that you need API nodes.",
													Type:        schema.TypeBool,
													Optional:    true,
													ForceNew:    true,
													Default:     defaultRonDBConfiguration("").Configuration.General.Benchmark.GrantUserPrivileges,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"management_nodes": {
				Description: "The configuration of RonDB management nodes.",
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"instance_type": {
							Description: fmt.Sprintf("The instance type of the RonDB management node. Defaults to %s for AWS and %s for Azure.", defaultRonDBConfiguration(api.AWS).ManagementNodes.InstanceType, defaultRonDBConfiguration(api.AZURE).ManagementNodes.InstanceType),
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Computed:    true,
						},
						"disk_size": {
							Description: "The disk size of management nodes in units of GB",
							Type:        schema.TypeInt,
							Optional:    true,
							ForceNew:    true,
							Default:     defaultRonDBConfiguration("").ManagementNodes.DiskSize,
						},
						"count": {
							Description:  "The number of management nodes.",
							Type:         schema.TypeInt,
							Optional:     true,
							ForceNew:     true,
							Default:      defaultRonDBConfiguration("").ManagementNodes.Count,
							ValidateFunc: validation.IntInSlice([]int{1}),
						},
					},
				},
			},
			"data_nodes": {
				Description: "The configuration of RonDB data nodes.",
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"instance_type": {
							Description: fmt.Sprintf("The instance type of the RonDB data node. Defaults to %s for AWS and %s for Azure.", defaultRonDBConfiguration(api.AWS).DataNodes.InstanceType, defaultRonDBConfiguration(api.AZURE).DataNodes.InstanceType),
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Computed:    true,
						},
						"disk_size": {
							Description: "The disk size of data nodes in units of GB",
							Type:        schema.TypeInt,
							Optional:    true,
							ForceNew:    true,
							Default:     defaultRonDBConfiguration("").DataNodes.DiskSize,
						},
						"count": {
							Description: "The number of data nodes. Notice that the number of RonDB data nodes have to be multiples of the replication_factor.",
							Type:        schema.TypeInt,
							Optional:    true,
							ForceNew:    true,
							Default:     defaultRonDBConfiguration("").DataNodes.Count,
						},
					},
				},
			},
			"mysql_nodes": {
				Description: "The configuration of MySQL nodes.",
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"instance_type": {
							Description: fmt.Sprintf("The instance type of the RonDB data node. Defaults to %s for AWS and %s for Azure.", defaultRonDBConfiguration(api.AWS).MYSQLNodes.InstanceType, defaultRonDBConfiguration(api.AZURE).MYSQLNodes.InstanceType),
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Computed:    true,
						},
						"disk_size": {
							Description: "The disk size of MySQL nodes in units of GB",
							Type:        schema.TypeInt,
							Optional:    true,
							ForceNew:    true,
							Default:     defaultRonDBConfiguration("").MYSQLNodes.DiskSize,
						},
						"count": {
							Description: "The number of MySQL nodes.",
							Type:        schema.TypeInt,
							Optional:    true,
							ForceNew:    true,
							Default:     defaultRonDBConfiguration("").MYSQLNodes.Count,
						},
					},
				},
			},
			"api_nodes": {
				Description: "The configuration of API nodes.",
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"instance_type": {
							Description: fmt.Sprintf("The instance type of the RonDB data node. Defaults to %s for AWS and %s for Azure.", defaultRonDBConfiguration(api.AWS).APINodes.InstanceType, defaultRonDBConfiguration(api.AZURE).APINodes.InstanceType),
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Computed:    true,
						},
						"disk_size": {
							Description: "The disk size of API nodes in units of GB",
							Type:        schema.TypeInt,
							Optional:    true,
							ForceNew:    true,
							Default:     defaultRonDBConfiguration("").APINodes.DiskSize,
						},
						"count": {
							Description: "The number of API nodes.",
							Type:        schema.TypeInt,
							Optional:    true,
							ForceNew:    true,
							Default:     defaultRonDBConfiguration("").APINodes.Count,
						},
					},
				},
			},
		},
	}
}

func autoscaleSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"instance_type": {
				Description: "The instance type to use while auto scaling.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"disk_size": {
				Description: "The disk size to use while auto scaling",
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     defaultAutoscaleConfiguration().DiskSize,
			},
			"min_workers": {
				Description:  "The minimum number of workers created by auto scaling.",
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      defaultAutoscaleConfiguration().MinWorkers,
				ValidateFunc: validation.IntAtLeast(0),
			},
			"max_workers": {
				Description: "The maximum number of workers created by auto scaling.",
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     defaultAutoscaleConfiguration().MaxWorkers,
			},
			"standby_workers": {
				Description:  "The percentage of workers to be always available during auto scaling. If you set this value to 0 new workers will only be added when a job or a notebook requests the resources. This attribute will not be taken into account if you set the minimum number of workers to 0 and no resources are used in the cluster, instead, it will start to take effect as soon as you start using resources.",
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      defaultAutoscaleConfiguration().StandbyWorkers,
				ValidateFunc: validation.FloatAtLeast(0),
			},
			"downscale_wait_time": {
				Description: "The time to wait before removing unused resources.",
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     defaultAutoscaleConfiguration().DownscaleWaitTime,
			},
			"spot_config": {
				Description: "The configuration to use spot instances",
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				MinItems:    1,
				Elem: &schema.Resource{
					Schema: spotSchema(),
				},
			},
		},
	}
}

func awsAttributesSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"region": {
				Description: "The AWS region where the cluster will be created.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"bucket_name": {
				Description: "The name of the S3 bucket that the cluster will use to store data in.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"instance_profile_arn": {
				Description:  "The ARN of the AWS instance profile that the cluster will be started with.",
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringMatch(instanceProfileRegex(), "You should use the Instance Profile ARNs"),
			},
			"network": {
				Description: "The network configurations.",
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"vpc_id": {
							Description: "The VPC id.",
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
						},
						"subnet_id": {
							Description: "The subnet id.",
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							ForceNew:    true,
						},
						"security_group_id": {
							Description: "The security group id.",
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							ForceNew:    true,
						},
					},
				},
			},
			"eks_cluster_name": {
				Description:  "The name of the AWS EKS cluster.",
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[0-9A-Za-z][A-Za-z0-9\-_]+$`), "Invalid EKS cluster name"),
			},
			"ecr_registry_account_id": {
				Description:  "The account id used for ECR. Defaults to the user's account id, inferred from the instance profille ARN.",
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringMatch(regexp.MustCompile(`^\d{12}$`), "Invalid ECR account id"),
			},
		},
	}
}

func azureAttributesSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"location": {
				Description: "The location where the cluster will be created.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"resource_group": {
				Description: "The resource group where the cluster will be created.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"storage_account": {
				Description: "The azure storage account that the cluster will use to store data in.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"storage_container_name": {
				Description: "The name of the azure storage container that the cluster will use to store data in. If not specified, it will be automatically generated.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
			},
			"user_assigned_managed_identity": {
				Description: "The azure user assigned managed identity that the cluster will be started with.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"network": {
				Description: "The network configurations.",
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"resource_group": {
							Description: "The resource group where the network resources reside. If not specified, the azure_attributes/resource_group will be used.",
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							ForceNew:    true,
						},
						"virtual_network_name": {
							Description: "The virtual network name.",
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
						},
						"subnet_name": {
							Description: "The subnet name.",
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
						},
						"security_group_name": {
							Description: "The security group name.",
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							ForceNew:    true,
						},
					},
				},
			},
			"aks_cluster_name": {
				Description: "The name of the AKS cluster.",
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
			},
			"acr_registry_name": {
				Description: "The name of the ACR registry.",
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
			},
		},
	}
}

func clusterResource() *schema.Resource {
	return &schema.Resource{
		Description:   "Use this resource to create, read, update, and delete clusters on Hopsworks.ai.",
		Schema:        clusterSchema(),
		CreateContext: resourceClusterCreate,
		ReadContext:   resourceClusterRead,
		UpdateContext: resourceClusterUpdate,
		DeleteContext: resourceClusterDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(45 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
			Update: schema.DefaultTimeout(45 * time.Minute),
		},
	}
}

func resourceClusterCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*api.HopsworksAIClient)

	if v, ok := d.GetOk("update_state"); ok && v != "none" {
		return diag.Errorf("you cannot update cluster state during creation")
	}

	baseRequest, err := createClusterBaseRequest(d)
	if err != nil {
		return diag.FromErr(err)
	}
	var createRequest interface{} = nil
	if aws, ok := d.GetOk("aws_attributes"); ok {
		awsAttributes := aws.([]interface{})
		if len(awsAttributes) > 0 {
			createRequest = createAWSCluster(awsAttributes[0].(map[string]interface{}), baseRequest)
		}
	}

	if azure, ok := d.GetOk("azure_attributes"); ok {
		azureAttributes := azure.([]interface{})
		if len(azureAttributes) > 0 {
			createRequest = createAzureCluster(azureAttributes[0].(map[string]interface{}), baseRequest)
		}
	}

	if createRequest == nil {
		return diag.Errorf("no request to create cluster")
	}

	clusterId, err := api.NewCluster(ctx, client, createRequest)
	if err != nil {
		return diag.Errorf("failed to create cluster, error: %s", err)
	}
	d.SetId(clusterId)
	if err := resourceClusterWaitForRunning(ctx, client, d.Timeout(schema.TimeoutCreate), clusterId); err != nil {
		return diag.FromErr(err)
	}

	if v, ok := d.GetOk("open_ports"); ok {
		openPortsArr := v.([]interface{})
		ports := structure.ExpandPorts(openPortsArr[0].(map[string]interface{}))
		if err := api.UpdateOpenPorts(ctx, client, clusterId, &ports); err != nil {
			return diag.Errorf("failed to open ports on cluster, error: %s", err)
		}
	}
	return resourceClusterRead(ctx, d, meta)
}

func createAWSCluster(awsAttributes map[string]interface{}, baseRequest *api.CreateCluster) *api.CreateAWSCluster {
	setAWSDefaults(baseRequest)
	req := api.CreateAWSCluster{
		CreateCluster: *baseRequest,
		AWSCluster: api.AWSCluster{
			Region:             awsAttributes["region"].(string),
			BucketName:         awsAttributes["bucket_name"].(string),
			InstanceProfileArn: awsAttributes["instance_profile_arn"].(string),
		},
	}

	if v, ok := awsAttributes["network"]; ok {
		networkArr := v.([]interface{})
		if len(networkArr) > 0 && networkArr[0] != nil {
			network := networkArr[0].(map[string]interface{})
			req.VpcId = network["vpc_id"].(string)
			req.SubnetId = network["subnet_id"].(string)
			req.SecurityGroupId = network["security_group_id"].(string)
		}
	}

	if v, ok := awsAttributes["eks_cluster_name"]; ok && v != "" {
		req.EksClusterName = v.(string)
		if registry, okR := awsAttributes["ecr_registry_account_id"]; okR && registry != "" {
			req.EcrRegistryAccountId = registry.(string)
		} else {
			submatches := instanceProfileRegex().FindStringSubmatch(req.InstanceProfileArn)
			if len(submatches) == 3 {
				req.EcrRegistryAccountId = submatches[1]
			}
		}
	}
	return &req
}

func createAzureCluster(azureAttributes map[string]interface{}, baseRequest *api.CreateCluster) *api.CreateAzureCluster {
	setAzureDefaults(baseRequest)
	containerName := azureAttributes["storage_container_name"].(string)
	if containerName == "" {
		suffix := time.Now().UnixNano() / 1e6
		containerName = fmt.Sprintf("hopsworksai-%d", suffix)
	}

	req := api.CreateAzureCluster{
		CreateCluster: *baseRequest,
		AzureCluster: api.AzureCluster{
			Location:          azureAttributes["location"].(string),
			ResourceGroup:     azureAttributes["resource_group"].(string),
			StorageAccount:    azureAttributes["storage_account"].(string),
			BlobContainerName: containerName,
			ManagedIdentity:   azureAttributes["user_assigned_managed_identity"].(string),
		},
	}

	if v, ok := azureAttributes["network"]; ok {
		networkArr := v.([]interface{})
		if len(networkArr) > 0 && networkArr[0] != nil {
			network := networkArr[0].(map[string]interface{})
			req.VirtualNetworkName = network["virtual_network_name"].(string)
			req.SubnetName = network["subnet_name"].(string)
			req.SecurityGroupName = network["security_group_name"].(string)
			req.NetworkResourceGroup = network["resource_group"].(string)
		}
	}

	if aks, ok := azureAttributes["aks_cluster_name"]; ok && aks != "" {
		req.AksClusterName = aks.(string)
		if registry, okR := azureAttributes["acr_registry_name"]; okR {
			req.AcrRegistryName = registry.(string)
		}
	}
	return &req
}

func createClusterBaseRequest(d *schema.ResourceData) (*api.CreateCluster, error) {
	headConfig := d.Get("head").([]interface{})[0].(map[string]interface{})
	tagsMap := d.Get("tags").(map[string]interface{})
	tagsArr := make([]api.ClusterTag, len(tagsMap))
	var index int = 0
	for k, v := range tagsMap {
		tagsArr[index] = api.ClusterTag{
			Name:  k,
			Value: v.(string),
		}
		index++
	}

	createCluster := &api.CreateCluster{
		Name:       d.Get("name").(string),
		Version:    d.Get("version").(string),
		SshKeyName: d.Get("ssh_key").(string),
		ClusterConfiguration: api.ClusterConfiguration{
			Head: api.HeadConfiguration{
				NodeConfiguration: structure.ExpandNode(headConfig),
			},
			Workers: []api.WorkerConfiguration{},
		},
		IssueLetsEncrypt:      d.Get("issue_lets_encrypt_certificate").(bool),
		AttachPublicIP:        d.Get("attach_public_ip").(bool),
		ManagedUsers:          d.Get("managed_users").(bool),
		BackupRetentionPeriod: d.Get("backup_retention_period").(int),
		Tags:                  tagsArr,
		InitScript:            d.Get("init_script").(string),
		OS:                    d.Get("os").(string),
	}

	if v, ok := d.GetOk("workers"); ok {
		vL := v.(*schema.Set).List()
		workersConfig := make([]api.WorkerConfiguration, 0)
		for _, w := range vL {
			config := w.(map[string]interface{})
			workersConfig = append(workersConfig, structure.ExpandWorker(config))
		}
		createCluster.ClusterConfiguration.Workers = workersConfig
	}

	if _, ok := d.GetOk("rondb"); ok {
		var cloud api.CloudProvider = ""
		if _, ok := d.GetOk("aws_attributes"); ok {
			cloud = api.AWS
		}

		if _, ok := d.GetOk("azure_attributes"); ok {
			cloud = api.AZURE
		}

		defaultRonDB := defaultRonDBConfiguration(cloud)

		var replicationFactor = defaultRonDB.Configuration.NdbdDefault.ReplicationFactor
		if v, ok := d.GetOk("rondb.0.configuration.0.ndbd_default.0.replication_factor"); ok {
			replicationFactor = v.(int)
		}

		var grantUserPrivileges = defaultRonDB.Configuration.General.Benchmark.GrantUserPrivileges
		if v, ok := d.GetOk("rondb.0.configuration.0.general.0.benchmark.0.grant_user_privileges"); ok {
			grantUserPrivileges = v.(bool)
		}

		createCluster.RonDB = &api.RonDBConfiguration{
			Configuration: api.RonDBBaseConfiguration{
				NdbdDefault: api.RonDBNdbdDefaultConfiguration{
					ReplicationFactor: replicationFactor,
				},
				General: api.RonDBGeneralConfiguration{
					Benchmark: api.RonDBBenchmarkConfiguration{
						GrantUserPrivileges: grantUserPrivileges,
					},
				},
			},
		}

		if n, ok := d.GetOk("rondb.0.management_nodes"); ok && len(n.([]interface{})) > 0 {
			createCluster.RonDB.ManagementNodes = structure.ExpandWorker(n.([]interface{})[0].(map[string]interface{}))
			if createCluster.RonDB.ManagementNodes.InstanceType == "" {
				createCluster.RonDB.ManagementNodes.InstanceType = defaultRonDB.ManagementNodes.InstanceType
			}
		} else {
			createCluster.RonDB.ManagementNodes = defaultRonDB.ManagementNodes
		}

		if n, ok := d.GetOk("rondb.0.data_nodes"); ok && len(n.([]interface{})) > 0 {
			createCluster.RonDB.DataNodes = structure.ExpandWorker(n.([]interface{})[0].(map[string]interface{}))
			if createCluster.RonDB.DataNodes.InstanceType == "" {
				createCluster.RonDB.DataNodes.InstanceType = defaultRonDB.DataNodes.InstanceType
			}
		} else {
			createCluster.RonDB.DataNodes = defaultRonDB.DataNodes
		}

		if n, ok := d.GetOk("rondb.0.mysql_nodes"); ok && len(n.([]interface{})) > 0 {
			createCluster.RonDB.MYSQLNodes = structure.ExpandWorker(n.([]interface{})[0].(map[string]interface{}))
			if createCluster.RonDB.MYSQLNodes.InstanceType == "" {
				createCluster.RonDB.MYSQLNodes.InstanceType = defaultRonDB.MYSQLNodes.InstanceType
			}
		} else {
			createCluster.RonDB.MYSQLNodes = defaultRonDB.MYSQLNodes
		}

		if n, ok := d.GetOk("rondb.0.api_nodes"); ok && len(n.([]interface{})) > 0 {
			createCluster.RonDB.APINodes = structure.ExpandWorker(n.([]interface{})[0].(map[string]interface{}))
			if createCluster.RonDB.APINodes.InstanceType == "" {
				createCluster.RonDB.APINodes.InstanceType = defaultRonDB.APINodes.InstanceType
			}
		} else {
			createCluster.RonDB.APINodes = defaultRonDB.APINodes
		}

		if createCluster.RonDB.DataNodes.Count%createCluster.RonDB.Configuration.NdbdDefault.ReplicationFactor != 0 {
			return nil, fmt.Errorf("number of RonDB data nodes must be multiples of RonDB replication factor")
		}
	}

	if _, ok := d.GetOk("autoscale"); ok {
		createCluster.Autoscale = &api.AutoscaleConfiguration{}

		if n, ok := d.GetOk("autoscale.0.non_gpu_workers"); ok && len(n.([]interface{})) > 0 {
			config := n.([]interface{})[0].(map[string]interface{})
			createCluster.Autoscale.NonGPU = structure.ExpandAutoscaleConfigurationBase(config)
		}

		if n, ok := d.GetOk("autoscale.0.gpu_workers"); ok && len(n.([]interface{})) > 0 {
			config := n.([]interface{})[0].(map[string]interface{})
			createCluster.Autoscale.GPU = structure.ExpandAutoscaleConfigurationBase(config)
		}
	}
	return createCluster, nil
}

func setAWSDefaults(createRequest *api.CreateCluster) {
	if createRequest.ClusterConfiguration.Head.InstanceType == "" {
		createRequest.ClusterConfiguration.Head.InstanceType = awsDefaultInstanceType
	}
}

func setAzureDefaults(createRequest *api.CreateCluster) {
	if createRequest.ClusterConfiguration.Head.InstanceType == "" {
		createRequest.ClusterConfiguration.Head.InstanceType = azureDefaultInstanceType
	}
}

func resourceClusterRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*api.HopsworksAIClient)
	id := d.Id()
	var diags diag.Diagnostics

	cluster, err := api.GetCluster(ctx, client, id)
	if err != nil {
		return diag.Errorf("failed to obtain cluster state: %s", err)
	}

	if cluster == nil {
		return diag.Errorf("cluster not found for cluster_id %s", id)
	}
	if err := populateClusterStateForResource(cluster, d); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func resourceClusterUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*api.HopsworksAIClient)

	clusterId := d.Id()

	if d.HasChange("workers") {
		o, n := d.GetChange("workers")
		old, new := o.(*schema.Set), n.(*schema.Set)

		oldWorkersMap := structure.ExpandWorkers(old)
		newWorkersMap := structure.ExpandWorkers(new)

		toAdd := make([]api.WorkerConfiguration, 0)
		toRemove := make([]api.WorkerConfiguration, 0)

		if len(newWorkersMap) == 0 {
			for _, v := range oldWorkersMap {
				toRemove = append(toRemove, v)
			}
		} else {
			for k, newWorker := range newWorkersMap {
				if oldWorker, found := oldWorkersMap[k]; found {
					if newWorker.Count > oldWorker.Count {
						toAdd = append(toAdd, api.WorkerConfiguration{
							NodeConfiguration: newWorker.NodeConfiguration,
							Count:             newWorker.Count - oldWorker.Count,
						})
					} else if newWorker.Count < oldWorker.Count {
						toRemove = append(toRemove, api.WorkerConfiguration{
							NodeConfiguration: newWorker.NodeConfiguration,
							Count:             oldWorker.Count - newWorker.Count,
						})
					}
					delete(oldWorkersMap, k)
				} else if newWorker.Count > 0 {
					toAdd = append(toAdd, newWorker)
				}
			}
			if len(oldWorkersMap) != 0 {
				for _, v := range oldWorkersMap {
					toRemove = append(toRemove, v)
				}
			}
		}

		log.Printf("[DEBUG] update workers \ntoAdd=%#v, \ntoRemove=%#v", toAdd, toRemove)
		if len(toRemove) > 0 {
			if err := api.RemoveWorkers(ctx, client, clusterId, toRemove); err != nil {
				return diag.FromErr(err)
			}
			if err := resourceClusterWaitForRunning(ctx, client, d.Timeout(schema.TimeoutUpdate), clusterId); err != nil {
				return diag.FromErr(err)
			}
		}

		if len(toAdd) > 0 {
			if err := api.AddWorkers(ctx, client, clusterId, toAdd); err != nil {
				return diag.FromErr(err)
			}
			if err := resourceClusterWaitForRunning(ctx, client, d.Timeout(schema.TimeoutUpdate), clusterId); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if d.HasChange("open_ports") {
		_, n := d.GetChange("open_ports")
		new := n.([]interface{})
		var ports api.ServiceOpenPorts = api.ServiceOpenPorts{}
		if len(new) != 0 {
			ports = structure.ExpandPorts(new[0].(map[string]interface{}))
		}
		if err := api.UpdateOpenPorts(ctx, client, clusterId, &ports); err != nil {
			return diag.Errorf("failed to open ports on cluster, error: %s", err)
		}
	}

	if d.HasChange("autoscale") {
		_, n := d.GetChange("autoscale")
		new := n.([]interface{})

		if len(new) == 0 {
			if err := api.DisableAutoscale(ctx, client, clusterId); err != nil {
				return diag.FromErr(err)
			}
		} else {
			newConfig := new[0].(map[string]interface{})

			autoscaleConfig := &api.AutoscaleConfiguration{}
			nonGpuConfig := newConfig["non_gpu_workers"].([]interface{})
			// required field
			autoscaleConfig.NonGPU = structure.ExpandAutoscaleConfigurationBase(nonGpuConfig[0].(map[string]interface{}))

			gpuConfig := newConfig["gpu_workers"].([]interface{})
			if len(gpuConfig) > 0 {
				autoscaleConfig.GPU = structure.ExpandAutoscaleConfigurationBase(gpuConfig[0].(map[string]interface{}))
			}

			if err := api.ConfigureAutoscale(ctx, client, clusterId, autoscaleConfig); err != nil {
				return diag.FromErr(err)
			}

			if err := resourceClusterWaitForRunning(ctx, client, d.Timeout(schema.TimeoutUpdate), clusterId); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if d.HasChange("update_state") {
		_, n := d.GetChange("update_state")
		new := n.(string)
		state := d.Get("state").(string)
		activationState := d.Get("activation_state").(string)

		if new == "start" {
			if activationState == api.Startable.String() {
				if err := api.StartCluster(ctx, client, clusterId); err != nil {
					return diag.Errorf("failed to start cluster: %s", err)
				}
				if err := resourceClusterWaitForRunning(ctx, client, d.Timeout(schema.TimeoutUpdate), clusterId); err != nil {
					return diag.FromErr(err)
				}
			} else {
				if state == api.Running.String() {
					return diag.Errorf("cluster is already running")
				} else {
					return diag.Errorf("cluster is not in startable state, current activation state is %s", activationState)
				}
			}
		} else if new == "stop" {
			if activationState == api.Stoppable.String() {
				if err := api.StopCluster(ctx, client, clusterId); err != nil {
					return diag.Errorf("failed to start cluster: %s", err)
				}
				if err := resourceClusterWaitForStopping(ctx, client, d.Timeout(schema.TimeoutUpdate), clusterId); err != nil {
					return diag.FromErr(err)
				}
			} else {
				if state == api.Stopped.String() {
					return diag.Errorf("cluster is already stopped")
				} else {
					return diag.Errorf("cluster is not in stoppable state, current activation state is %s", activationState)
				}
			}
		}
	}

	return resourceClusterRead(ctx, d, meta)
}

func resourceClusterDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*api.HopsworksAIClient)
	id := d.Id()
	var diags diag.Diagnostics

	if err := api.DeleteCluster(ctx, client, id); err != nil {
		return diag.Errorf("failed to delete cluster, error: %s", err)
	}

	if err := resourceClusterWaitForDeleting(ctx, client, d.Timeout(schema.TimeoutDelete), id); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceClusterWaitForRunning(ctx context.Context, client *api.HopsworksAIClient, timeout time.Duration, clusterId string) error {
	waitUntilRunning := helpers.ClusterStateChange(
		[]api.ClusterState{
			api.Starting,
			api.Pending,
			api.Initializing,
			api.Updating,
			api.Decommissioning,
			api.WorkerStarting,
			api.WorkerPending,
			api.WorkerInitializing,
			api.WorkerShuttingdown,
			api.WorkerDecommissioning,
			api.RonDBInitializing,
			api.StartingHopsworks,
		},
		[]api.ClusterState{
			api.Running,
			api.Error,
			api.WorkerError,
		},
		timeout,
		func() (result interface{}, state string, err error) {
			cluster, err := api.GetCluster(ctx, client, clusterId)
			if err != nil {
				return nil, "", err
			}
			if cluster == nil {
				return nil, "", fmt.Errorf("cluster not found for cluster id %s", clusterId)
			}
			log.Printf("[INFO] polled cluster state: %s, stage: %s", cluster.State, cluster.InitializationStage)
			return cluster, cluster.State.String(), nil
		},
	)

	resp, err := waitUntilRunning.WaitForStateContext(ctx)
	if err != nil {
		return err
	}

	cluster := resp.(*api.Cluster)
	if cluster.State != api.Running {
		return fmt.Errorf("failed while waiting for the cluster to reach running state: %s", cluster.ErrorMessage)
	}
	return nil
}

func resourceClusterWaitForStopping(ctx context.Context, client *api.HopsworksAIClient, timeout time.Duration, clusterId string) error {
	waitUntilStopping := helpers.ClusterStateChange(
		[]api.ClusterState{
			api.Running,
			api.Pending,
			api.Stopping,
		},
		[]api.ClusterState{
			api.Stopped,
			api.Error,
		},
		timeout,
		func() (result interface{}, state string, err error) {
			cluster, err := api.GetCluster(ctx, client, clusterId)
			if err != nil {
				return nil, "", err
			}
			if cluster == nil {
				return nil, "", fmt.Errorf("cluster not found for cluster id %s", clusterId)
			}
			log.Printf("[INFO] polled cluster state: %s, stage: %s", cluster.State, cluster.InitializationStage)
			return cluster, cluster.State.String(), nil
		},
	)

	resp, err := waitUntilStopping.WaitForStateContext(ctx)
	if err != nil {
		return err
	}

	cluster := resp.(*api.Cluster)
	if cluster.State != api.Stopped {
		return fmt.Errorf("failed to stop cluster, error: %s", cluster.ErrorMessage)
	}
	return nil
}

func resourceClusterWaitForDeleting(ctx context.Context, client *api.HopsworksAIClient, timeout time.Duration, clusterId string) error {
	waitUntilDeleted := helpers.ClusterStateChange(
		[]api.ClusterState{
			api.Running,
			api.ShuttingDown,
		},
		[]api.ClusterState{
			api.Error,
			api.ClusterDeleted,
			api.TerminationWarning,
		},
		timeout,
		func() (result interface{}, state string, err error) {
			cluster, err := api.GetCluster(ctx, client, clusterId)
			if err != nil {
				return nil, "", err
			}
			if cluster == nil {
				log.Printf("[DEBUG] cluster (id: %s) is not found", clusterId)
				return api.Cluster{Id: ""}, api.ClusterDeleted.String(), nil
			}
			log.Printf("[INFO] polled cluster state: %s, stage: %s", cluster.State, cluster.InitializationStage)
			return cluster, cluster.State.String(), nil
		},
	)

	resp, err := waitUntilDeleted.WaitForStateContext(ctx)
	if err != nil {
		return err
	}

	if resp != nil && resp.(api.Cluster).Id != "" {
		return fmt.Errorf("failed to delete cluster, error: %s", resp.(*api.Cluster).ErrorMessage)
	}
	return nil
}

func populateClusterStateForResource(cluster *api.Cluster, d *schema.ResourceData) error {
	d.SetId(cluster.Id)
	for k, v := range structure.FlattenCluster(cluster) {
		if _, ok := d.GetOk(k); ok && k == "update_state" {
			continue
		}
		if err := d.Set(k, v); err != nil {
			return err
		}
	}
	return nil
}
