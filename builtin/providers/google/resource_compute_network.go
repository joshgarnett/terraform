package google

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

func resourceComputeNetwork() *schema.Resource {
	return &schema.Resource{
		Create: resourceComputeNetworkCreate,
		Read:   resourceComputeNetworkRead,
		Delete: resourceComputeNetworkDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"ipv4_range": &schema.Schema{
				Type:       schema.TypeString,
				Optional:   true,
				ForceNew:   true,
				Deprecated: "Please use google_compute_subnetwork resources instead.",
			},

			"gateway_ipv4": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"auto_create_subnetworks": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				/* Ideally this would default to true as per the API, but that would cause
				   existing Terraform configs which have not been updated to report this as
				   a change. Perhaps we can bump this for a minor release bump rather than
				   a point release. */
				Default: false,
			},

			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"self_link": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceComputeNetworkCreate(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)

	//
	// Possible modes:
	// - 1 Legacy mode - Create a network in the legacy mode. ipv4_range is set. auto_create_subnetworks must be false
	//     and not sent in request
	// - 2 Distributed Mode - Create a new generation network that supports subnetworks:
	//   - 2.a - Auto subnet mode - auto_create_subnetworks = true, Google will generate 1 subnetwork per region
	//   - 2.b - Custom subnet mode - auto_create_subnetworks = false & ipv4_range not set,
	//
	ipv4range := d.Get("ipv4_range").(string)
	autoCreateSubnetworks := d.Get("auto_create_subnetworks").(bool)

	if ipv4range != "" && autoCreateSubnetworks {
		return fmt.Errorf("Error: cannot define ipv4_range with auto_create_subnetworks = true.")
	}

	// Build the network parameter
	network := &compute.Network{
		Name: d.Get("name").(string),
		AutoCreateSubnetworks: autoCreateSubnetworks,
		Description:           d.Get("description").(string),
	}

	if v, ok := d.GetOk("ipv4_range"); ok {
		log.Printf("[DEBUG] Setting IPv4Range (%#v) for legacy network mode", v.(string))
		network.IPv4Range = v.(string)
	} else {
		// custom subnet mode, so make sure AutoCreateSubnetworks field is included in request otherwise
		// google will create a network in legacy mode.
		network.ForceSendFields = []string{"AutoCreateSubnetworks"}
	}

	log.Printf("[DEBUG] Network insert request: %#v", network)
	op, err := config.clientCompute.Networks.Insert(
		config.Project, network).Do()
	if err != nil {
		return fmt.Errorf("Error creating network: %s", err)
	}

	// It probably maybe worked, so store the ID now
	d.SetId(network.Name)

	err = computeOperationWaitGlobal(config, op, "Creating Network")
	if err != nil {
		return err
	}

	return resourceComputeNetworkRead(d, meta)
}

func resourceComputeNetworkRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)

	network, err := config.clientCompute.Networks.Get(
		config.Project, d.Id()).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == 404 {
			log.Printf("[WARN] Removing Network %q because it's gone", d.Get("name").(string))
			// The resource doesn't exist anymore
			d.SetId("")

			return nil
		}

		return fmt.Errorf("Error reading network: %s", err)
	}

	d.Set("gateway_ipv4", network.GatewayIPv4)
	d.Set("self_link", network.SelfLink)

	return nil
}

func resourceComputeNetworkDelete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)

	// Delete the network
	op, err := config.clientCompute.Networks.Delete(
		config.Project, d.Id()).Do()
	if err != nil {
		return fmt.Errorf("Error deleting network: %s", err)
	}

	err = computeOperationWaitGlobal(config, op, "Deleting Network")
	if err != nil {
		return err
	}

	d.SetId("")
	return nil
}
