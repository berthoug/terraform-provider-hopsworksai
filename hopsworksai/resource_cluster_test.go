package hopsworksai

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/logicalclocks/terraform-provider-hopsworksai/hopsworksai/internal/api"
)

func TestAccClusterAWS_basic(t *testing.T) {
	testAccCluster_basic(t, api.AWS)
}

func TestAccClusterAZURE_basic(t *testing.T) {
	testAccCluster_basic(t, api.AZURE)
}

func testAccCluster_basic(t *testing.T, cloud api.CloudProvider) {
	suffix := acctest.RandString(5)
	rName := fmt.Sprintf("test_%s", suffix)
	resourceName := fmt.Sprintf("hopsworksai_cluster.%s", rName)
	parallelTest(t, cloud, resource.TestCase{
		PreCheck:     testAccPreCheck(t),
		Providers:    testAccProviders,
		CheckDestroy: testAccClusterCheckDestroy(),
		Steps: []resource.TestStep{
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "url"),
					resource.TestCheckResourceAttr(resourceName, "state", api.Running.String()),
					resource.TestCheckResourceAttr(resourceName, "activation_state", api.Stoppable.String()),
					resource.TestCheckResourceAttr(resourceName, "update_state", "none"),
					resource.TestCheckResourceAttr(resourceName, "workers.#", "0"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.ssh", "false"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.kafka", "false"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.feature_store", "false"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.online_feature_store", "false"),

					resource.TestCheckResourceAttr(resourceName, strings.ToLower(cloud.String())+"_attributes.0.network.#", "1"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config:             testAccClusterConfig_basic(cloud, rName, suffix, `update_state = "start"`),
				ExpectError:        regexp.MustCompile("cluster is already running"),
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, fmt.Sprintf(`
				workers{
					instance_type = "%s"
					disk_size = 256
					count = 2
				}`, testWorkerInstanceType1(cloud))),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "state", api.Running.String()),
					resource.TestCheckResourceAttr(resourceName, "activation_state", api.Stoppable.String()),
					resource.TestCheckResourceAttr(resourceName, "update_state", "none"),
					resource.TestCheckResourceAttr(resourceName, "workers.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType1(cloud),
						"disk_size":     "256",
						"count":         "2",
					}),
				),
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, fmt.Sprintf(`
				workers{
					instance_type = "%s"
					disk_size = 256
					count = 1
				}
				workers{
					instance_type = "%s"
					disk_size = 512
					count = 1
				}
				`, testWorkerInstanceType1(cloud), testWorkerInstanceType1(cloud))),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "state", api.Running.String()),
					resource.TestCheckResourceAttr(resourceName, "activation_state", api.Stoppable.String()),
					resource.TestCheckResourceAttr(resourceName, "update_state", "none"),
					resource.TestCheckResourceAttr(resourceName, "workers.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType1(cloud),
						"disk_size":     "256",
						"count":         "1",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType1(cloud),
						"disk_size":     "512",
						"count":         "1",
					}),
				),
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, fmt.Sprintf(`
				workers{
					instance_type = "%s"
					disk_size = 256
					count = 1
				}
				workers{
					instance_type = "%s"
					disk_size = 512
					count = 1
				}
				workers{
					instance_type = "%s"
					disk_size = 512
					count = 1
				}
				`, testWorkerInstanceType1(cloud), testWorkerInstanceType1(cloud), testWorkerInstanceType2(cloud))),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "state", api.Running.String()),
					resource.TestCheckResourceAttr(resourceName, "activation_state", api.Stoppable.String()),
					resource.TestCheckResourceAttr(resourceName, "update_state", "none"),
					resource.TestCheckResourceAttr(resourceName, "workers.#", "3"),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType1(cloud),
						"disk_size":     "256",
						"count":         "1",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType1(cloud),
						"disk_size":     "512",
						"count":         "1",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType2(cloud),
						"disk_size":     "512",
						"count":         "1",
					}),
				),
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, fmt.Sprintf(`
				workers{
					instance_type = "%s"
					disk_size = 512
					count = 1
				}
				`, testWorkerInstanceType2(cloud))),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "state", api.Running.String()),
					resource.TestCheckResourceAttr(resourceName, "activation_state", api.Stoppable.String()),
					resource.TestCheckResourceAttr(resourceName, "update_state", "none"),
					resource.TestCheckResourceAttr(resourceName, "workers.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType2(cloud),
						"disk_size":     "512",
						"count":         "1",
					}),
				),
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, fmt.Sprintf(`
				workers{
					instance_type = "%s"
					disk_size = 512
					count = 2
				}
				workers{
					instance_type = "%s"
					disk_size = 512
					count = 1
				}
				`, testWorkerInstanceType2(cloud), testWorkerInstanceType1(cloud))),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "state", api.Running.String()),
					resource.TestCheckResourceAttr(resourceName, "activation_state", api.Stoppable.String()),
					resource.TestCheckResourceAttr(resourceName, "update_state", "none"),
					resource.TestCheckResourceAttr(resourceName, "workers.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType2(cloud),
						"disk_size":     "512",
						"count":         "2",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType1(cloud),
						"disk_size":     "512",
						"count":         "1",
					}),
				),
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, fmt.Sprintf(`
				workers{
					instance_type = "%s"
					disk_size = 512
					count = 1
				}
				`, testWorkerInstanceType2(cloud))),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "state", api.Running.String()),
					resource.TestCheckResourceAttr(resourceName, "activation_state", api.Stoppable.String()),
					resource.TestCheckResourceAttr(resourceName, "update_state", "none"),
					resource.TestCheckResourceAttr(resourceName, "workers.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "workers.*", map[string]string{
						"instance_type": testWorkerInstanceType2(cloud),
						"disk_size":     "512",
						"count":         "1",
					}),
				),
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, `
				open_ports{
					ssh = true
					kafka = true
				}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "open_ports.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.ssh", "true"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.kafka", "true"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.feature_store", "false"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.online_feature_store", "false"),
				),
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, `
				open_ports{
					feature_store = true
					online_feature_store = true
				}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "open_ports.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.ssh", "false"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.kafka", "false"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.feature_store", "true"),
					resource.TestCheckResourceAttr(resourceName, "open_ports.0.online_feature_store", "true"),
				),
			},
			{
				Config: testAccClusterConfig_basic(cloud, rName, suffix, `update_state = "stop"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "state", api.Stopped.String()),
					resource.TestCheckResourceAttr(resourceName, "activation_state", api.Startable.String()),
					resource.TestCheckResourceAttr(resourceName, "update_state", "stop"),
				),
			},
		},
	})
}

func testWorkerInstanceType1(cloud api.CloudProvider) string {
	return testWorkerInstanceType(cloud, true)
}

func testWorkerInstanceType2(cloud api.CloudProvider) string {
	return testWorkerInstanceType(cloud, false)
}

func testWorkerInstanceType(cloud api.CloudProvider, alternative bool) string {
	if cloud == api.AWS {
		if alternative {
			return "t3a.medium"
		} else {
			return "t3a.large"
		}
	} else if cloud == api.AZURE {
		if alternative {
			return "Standard_D4_v3"
		} else {
			return "Standard_D8_v3"
		}
	}
	return ""
}

func testAccClusterCheckDestroy() func(s *terraform.State) error {
	return func(s *terraform.State) error {
		client := testAccProvider.Meta().(*api.HopsworksAIClient)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "hopsworksai_cluster" {
				continue
			}
			cluster, err := api.GetCluster(context.Background(), client, rs.Primary.ID)
			if err != nil {
				return err
			}

			if cluster != nil {
				return fmt.Errorf("found unterminated cluster %s", rs.Primary.ID)
			}
		}
		return nil
	}
}

func testAccClusterConfig_basic(cloud api.CloudProvider, rName string, suffix string, extraConfig string) string {
	return testAccClusterConfig(cloud, rName, suffix, extraConfig, 0)
}

func testAccClusterConfig(cloud api.CloudProvider, rName string, suffix string, extraConfig string, bucketIndex int) string {
	return fmt.Sprintf(`
	resource "hopsworksai_cluster" "%s" {
		name    = "%s%s%s"
		ssh_key = "%s"	  
		head {
		}
		
		%s
		
		%s 

		tags = {
		  "Purpose" = "acceptance-test"
		}
	  }
	`,
		rName,
		clusterPrefixName,
		strings.ToLower(cloud.String()),
		suffix,
		testAccClusterCloudSSHKeyAttribute(cloud),
		testAccClusterCloudConfigAttributes(cloud, bucketIndex),
		extraConfig)
}
