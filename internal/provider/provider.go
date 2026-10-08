// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zack/terraform-provider-hetzner/internal/client"
)

var (
	_ provider.Provider                       = &HetznerProvider{}
	_ provider.ProviderWithEphemeralResources = &HetznerProvider{}
)

// HetznerProvider defines the provider implementation.
type HetznerProvider struct {
	version string
}

// HetznerProviderModel describes the provider data model.
type HetznerProviderModel struct {
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

func (p *HetznerProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hetzner"
	resp.Version = p.version
}

func (p *HetznerProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Hetzner provider allows managing resources in the Hetzner Robot API.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				MarkdownDescription: "Username for Hetzner Robot API. Can also be set via the `HETZNER_ROBOT_USERNAME` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for Hetzner Robot API. Can also be set via the `HETZNER_ROBOT_PASSWORD` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *HetznerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data HetznerProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve username: config takes precedence over environment variable.
	username := os.Getenv("HETZNER_ROBOT_USERNAME")
	if !data.Username.IsNull() {
		username = data.Username.ValueString()
	}

	password := os.Getenv("HETZNER_ROBOT_PASSWORD")
	if !data.Password.IsNull() {
		password = data.Password.ValueString()
	}

	if username == "" {
		resp.Diagnostics.AddError(
			"Missing Hetzner Robot Username",
			"The provider requires a username for the Hetzner Robot API. "+
				"Set the username in the provider configuration or via the HETZNER_ROBOT_USERNAME environment variable.",
		)
	}

	if password == "" {
		resp.Diagnostics.AddError(
			"Missing Hetzner Robot Password",
			"The provider requires a password for the Hetzner Robot API. "+
				"Set the password in the provider configuration or via the HETZNER_ROBOT_PASSWORD environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	c := client.NewClient(username, password)

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *HetznerProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// Batch 1: SSH keys, rDNS, firewall
		NewSSHKeyResource,
		NewRDNSResource,
		NewFirewallResource,
		NewFirewallTemplateResource,
		// Batch 2: vSwitch, IP, subnet, failover
		NewVSwitchResource,
		NewVSwitchServerResource,
		NewIPResource,
		NewSubnetResource,
		NewFailoverResource,
		// Batch 4: server ordering
		NewServerOrderResource,
		NewServerNameResource,
		NewServerAddonResource,
		// Batch 3: boot configs
		NewBootRescueResource,
		NewBootLinuxResource,
		NewBootVNCResource,
		NewBootWindowsResource,
		// Batch 5: MAC address management, cancellations
		NewIPMACResource,
		NewSubnetMACResource,
		NewIPCancellationResource,
		NewSubnetCancellationResource,
	}
}

func (p *HetznerProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// Batch 1: SSH keys, rDNS, firewall
		NewSSHKeyDataSource,
		NewSSHKeysDataSource,
		NewRDNSDataSource,
		NewFirewallDataSource,
		NewFirewallTemplateDataSource,
		NewFirewallTemplatesDataSource,
		// Batch 2: vSwitch, server, IP, subnet, failover
		NewVSwitchDataSource,
		NewVSwitchesDataSource,
		NewServerDataSource,
		NewServersDataSource,
		NewIPDataSource,
		NewIPsDataSource,
		NewSubnetDataSource,
		NewSubnetsDataSource,
		NewFailoverDataSource,
		NewFailoversDataSource,
		// Batch 4: order/market products, server addons
		NewServerOrderProductsDataSource,
		NewServerMarketProductsDataSource,
		NewServerAddonsDataSource,
		// Batch 3: boot configs, reset, WoL
		NewBootRescueDataSource,
		NewBootLinuxDataSource,
		NewBootVNCDataSource,
		NewBootWindowsDataSource,
		NewResetDataSource,
		NewWOLDataSource,
		// Batch 5: traffic, rDNS list
		NewTrafficDataSource,
		NewRDNSListDataSource,
	}
}

func (p *HetznerProvider) EphemeralResources(ctx context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		NewResetEphemeralResource,
		NewWOLEphemeralResource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &HetznerProvider{
			version: version,
		}
	}
}
