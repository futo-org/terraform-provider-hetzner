// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zack/terraform-provider-hetzner/internal/client"
)

var (
	_ resource.Resource                   = &bootLinuxResource{}
	_ resource.ResourceWithConfigure      = &bootLinuxResource{}
	_ resource.ResourceWithImportState    = &bootLinuxResource{}
	_ resource.ResourceWithValidateConfig = &bootLinuxResource{}
)

type bootLinuxResource struct {
	client *client.Client
}

type bootLinuxResourceModel struct {
	ServerNumber   types.Int64  `tfsdk:"server_number"`
	Dist           types.String `tfsdk:"dist"`
	Lang           types.String `tfsdk:"lang"`
	Arch           types.Int64  `tfsdk:"arch"`
	AuthorizedKey  types.String `tfsdk:"authorized_key"`
	AuthorizedKeys types.List   `tfsdk:"authorized_keys"`
	ServerIP       types.String `tfsdk:"server_ip"`
	ServerIPv6Net  types.String `tfsdk:"server_ipv6_net"`
	Active         types.Bool   `tfsdk:"active"`
	Password       types.String `tfsdk:"password"`
}

type linuxAPIResponse struct {
	Linux linuxAPIData `json:"linux"`
}

type linuxAPIData struct {
	ServerIP      string      `json:"server_ip"`
	ServerIPv6Net string      `json:"server_ipv6_net"`
	ServerNumber  int         `json:"server_number"`
	Dist          interface{} `json:"dist"`
	Lang          interface{} `json:"lang"`
	Active        bool        `json:"active"`
	Password      *string     `json:"password"`
}

func NewBootLinuxResource() resource.Resource {
	return &bootLinuxResource{}
}

func (r *bootLinuxResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_boot_linux"
}

func (r *bootLinuxResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Linux installation boot configuration for a Hetzner dedicated server.",
		Attributes: map[string]schema.Attribute{
			"server_number": schema.Int64Attribute{
				MarkdownDescription: "The server number.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"dist": schema.StringAttribute{
				MarkdownDescription: "Linux distribution.",
				Required:            true,
			},
			"lang": schema.StringAttribute{
				MarkdownDescription: "Language.",
				Required:            true,
			},
			"arch": schema.Int64Attribute{
				MarkdownDescription: "Architecture (64 or 32). Defaults to 64.",
				Optional:            true,
			},
			"authorized_key": schema.StringAttribute{
				MarkdownDescription: "SSH key fingerprint for authentication. Conflicts with `authorized_keys`.",
				Optional:            true,
			},
			"authorized_keys": schema.ListAttribute{
				MarkdownDescription: "SSH key fingerprints for authentication. Conflicts with `authorized_key`.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"server_ip": schema.StringAttribute{
				MarkdownDescription: "Server main IPv4 address.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_ipv6_net": schema.StringAttribute{
				MarkdownDescription: "Server IPv6 network.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether Linux install is currently active.",
				Computed:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Generated password. Only available on activation.",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (r *bootLinuxResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Data", "Expected *client.Client")
		return
	}
	r.client = c
}

func (r *bootLinuxResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config bootLinuxResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	checkAuthorizedKeysConflict(config.AuthorizedKey, config.AuthorizedKeys, &resp.Diagnostics)
}

func linuxForm(ctx context.Context, plan bootLinuxResourceModel) (url.Values, diag.Diagnostics) {
	data := url.Values{}
	data.Set("dist", plan.Dist.ValueString())
	data.Set("lang", plan.Lang.ValueString())

	if !plan.Arch.IsNull() && !plan.Arch.IsUnknown() {
		data.Set("arch", strconv.FormatInt(plan.Arch.ValueInt64(), 10))
	}
	return data, addAuthorizedKeys(ctx, data, plan.AuthorizedKey, plan.AuthorizedKeys)
}

func (r *bootLinuxResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan bootLinuxResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverNum := plan.ServerNumber.ValueInt64()
	data, diags := linuxForm(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := r.client.Post(fmt.Sprintf("/boot/%d/linux", serverNum), data)
	if err != nil {
		resp.Diagnostics.AddError("Error activating Linux install", err.Error())
		return
	}

	var apiResp linuxAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		resp.Diagnostics.AddError("Error parsing Linux response", err.Error())
		return
	}

	plan.ServerIP = stringOrNull(apiResp.Linux.ServerIP)
	plan.ServerIPv6Net = types.StringValue(apiResp.Linux.ServerIPv6Net)
	plan.Active = types.BoolValue(apiResp.Linux.Active)

	if apiResp.Linux.Password != nil {
		plan.Password = types.StringValue(*apiResp.Linux.Password)
	} else {
		plan.Password = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bootLinuxResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state bootLinuxResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverNum := state.ServerNumber.ValueInt64()
	body, err := r.client.Get(fmt.Sprintf("/boot/%d/linux", serverNum))
	if err != nil {
		resp.Diagnostics.AddError("Error reading Linux boot config", err.Error())
		return
	}

	var apiResp linuxAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		resp.Diagnostics.AddError("Error parsing Linux response", err.Error())
		return
	}

	state.ServerIP = stringOrNull(apiResp.Linux.ServerIP)
	state.ServerIPv6Net = types.StringValue(apiResp.Linux.ServerIPv6Net)
	state.Active = types.BoolValue(apiResp.Linux.Active)

	if apiResp.Linux.Active {
		if distStr, ok := apiResp.Linux.Dist.(string); ok {
			state.Dist = types.StringValue(distStr)
		}
		if langStr, ok := apiResp.Linux.Lang.(string); ok {
			state.Lang = types.StringValue(langStr)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *bootLinuxResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan bootLinuxResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverNum := plan.ServerNumber.ValueInt64()
	data, diags := linuxForm(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Deactivate first
	_, err := r.client.Delete(fmt.Sprintf("/boot/%d/linux", serverNum))
	if err != nil {
		resp.Diagnostics.AddError("Error deactivating Linux install", err.Error())
		return
	}

	// Reactivate with new settings

	body, err := r.client.Post(fmt.Sprintf("/boot/%d/linux", serverNum), data)
	if err != nil {
		resp.Diagnostics.AddError("Error reactivating Linux install", err.Error())
		return
	}

	var apiResp linuxAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		resp.Diagnostics.AddError("Error parsing Linux response", err.Error())
		return
	}

	plan.ServerIP = stringOrNull(apiResp.Linux.ServerIP)
	plan.ServerIPv6Net = types.StringValue(apiResp.Linux.ServerIPv6Net)
	plan.Active = types.BoolValue(apiResp.Linux.Active)

	if apiResp.Linux.Password != nil {
		plan.Password = types.StringValue(*apiResp.Linux.Password)
	} else {
		plan.Password = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bootLinuxResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state bootLinuxResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.Delete(fmt.Sprintf("/boot/%d/linux", state.ServerNumber.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error deactivating Linux install", err.Error())
		return
	}
}

func (r *bootLinuxResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	serverNum, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected numeric server_number")
		return
	}

	var state bootLinuxResourceModel
	state.ServerNumber = types.Int64Value(serverNum)
	state.Dist = types.StringNull()
	state.Lang = types.StringNull()
	state.Arch = types.Int64Null()
	state.AuthorizedKey = types.StringNull()
	state.AuthorizedKeys = types.ListNull(types.StringType)
	state.ServerIP = types.StringNull()
	state.ServerIPv6Net = types.StringNull()
	state.Active = types.BoolNull()
	state.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
