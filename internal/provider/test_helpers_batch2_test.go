// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http/httptest"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/zack/terraform-provider-hetzner/internal/client"
)

var _ provider.Provider = &batch2TestProvider{}

// batch2TestProvider is a test-only provider for batch 2 resources (vSwitch, server, IP, subnet, failover).
type batch2TestProvider struct {
	client *client.Client
}

func newBatch2TestProvider(c *client.Client) func() provider.Provider {
	return func() provider.Provider {
		return &batch2TestProvider{client: c}
	}
}

func batch2ProviderFactories(ts *httptest.Server) map[string]func() (tfprotov6.ProviderServer, error) {
	c := client.NewClient("test", "test")
	c.BaseURL = ts.URL
	return map[string]func() (tfprotov6.ProviderServer, error){
		"hetzner": providerserver.NewProtocol6WithError(newBatch2TestProvider(c)()),
	}
}

func (p *batch2TestProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hetzner"
	resp.Version = "test"
}

func (p *batch2TestProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{Optional: true, Sensitive: true},
			"password": schema.StringAttribute{Optional: true, Sensitive: true},
		},
	}
}

func (p *batch2TestProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	resp.DataSourceData = p.client
	resp.ResourceData = p.client
}

func (p *batch2TestProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVSwitchResource,
		NewVSwitchServerResource,
		NewIPResource,
		NewSubnetResource,
		NewFailoverResource,
		NewServerOrderResource,
		NewServerNameResource,
		NewServerAddonResource,
		NewIPMACResource,
		NewSubnetMACResource,
		NewIPCancellationResource,
		NewSubnetCancellationResource,
	}
}

func (p *batch2TestProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
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
		NewServerOrderProductsDataSource,
		NewServerMarketProductsDataSource,
		NewServerAddonsDataSource,
		NewTrafficDataSource,
		NewRDNSListDataSource,
	}
}
