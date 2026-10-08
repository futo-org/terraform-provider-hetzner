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
	_ resource.Resource                   = &bootRescueResource{}
	_ resource.ResourceWithConfigure      = &bootRescueResource{}
	_ resource.ResourceWithImportState    = &bootRescueResource{}
	_ resource.ResourceWithValidateConfig = &bootRescueResource{}
)

type bootRescueResource struct {
	client *client.Client
}

type bootRescueResourceModel struct {
	ServerNumber   types.Int64  `tfsdk:"server_number"`
	OS             types.String `tfsdk:"os"`
	Arch           types.Int64  `tfsdk:"arch"`
	AuthorizedKey  types.String `tfsdk:"authorized_key"`
	AuthorizedKeys types.List   `tfsdk:"authorized_keys"`
	Keyboard       types.String `tfsdk:"keyboard"`
	ServerIP       types.String `tfsdk:"server_ip"`
	ServerIPv6Net  types.String `tfsdk:"server_ipv6_net"`
	Active         types.Bool   `tfsdk:"active"`
	Password       types.String `tfsdk:"password"`
}

func NewBootRescueResource() resource.Resource {
	return &bootRescueResource{}
}

func (r *bootRescueResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_boot_rescue"
}

func (r *bootRescueResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the rescue system boot configuration for a Hetzner dedicated server.",
		Attributes: map[string]schema.Attribute{
			"server_number": schema.Int64Attribute{
				MarkdownDescription: "The server number.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"os": schema.StringAttribute{
				MarkdownDescription: "Operating system for rescue mode (e.g. `linux`, `vkvm`).",
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
			"keyboard": schema.StringAttribute{
				MarkdownDescription: "Keyboard layout. Defaults to `us`.",
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
				MarkdownDescription: "Whether rescue mode is currently active.",
				Computed:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Generated password for rescue mode. Only available on activation.",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (r *bootRescueResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *bootRescueResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config bootRescueResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !config.AuthorizedKey.IsNull() && !config.AuthorizedKeys.IsNull() {
		resp.Diagnostics.AddAttributeError(frameworkPath("authorized_keys"), "Conflicting attributes",
			"Set either authorized_key or authorized_keys, not both.")
	}
}

func rescueForm(ctx context.Context, plan bootRescueResourceModel) (url.Values, diag.Diagnostics) {
	var diags diag.Diagnostics
	data := url.Values{}
	data.Set("os", plan.OS.ValueString())

	if !plan.Arch.IsNull() && !plan.Arch.IsUnknown() {
		data.Set("arch", strconv.FormatInt(plan.Arch.ValueInt64(), 10))
	}
	if !plan.AuthorizedKey.IsNull() && !plan.AuthorizedKey.IsUnknown() {
		data.Set("authorized_key", plan.AuthorizedKey.ValueString())
	}
	if !plan.AuthorizedKeys.IsNull() && !plan.AuthorizedKeys.IsUnknown() {
		var keys []string
		diags.Append(plan.AuthorizedKeys.ElementsAs(ctx, &keys, false)...)
		for _, k := range keys {
			data.Add("authorized_key[]", k)
		}
	}
	if !plan.Keyboard.IsNull() && !plan.Keyboard.IsUnknown() {
		data.Set("keyboard", plan.Keyboard.ValueString())
	}
	return data, diags
}

type rescueAPIResponse struct {
	Rescue rescueAPIData `json:"rescue"`
}

type rescueAPIData struct {
	ServerIP      string      `json:"server_ip"`
	ServerIPv6Net string      `json:"server_ipv6_net"`
	ServerNumber  int         `json:"server_number"`
	OS            interface{} `json:"os"`
	Active        bool        `json:"active"`
	Password      *string     `json:"password"`
	Keyboard      string      `json:"keyboard"`
}

func (r *bootRescueResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan bootRescueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverNum := plan.ServerNumber.ValueInt64()
	data, diags := rescueForm(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := r.client.Post(fmt.Sprintf("/boot/%d/rescue", serverNum), data)
	if err != nil {
		resp.Diagnostics.AddError("Error activating rescue system", err.Error())
		return
	}

	var apiResp rescueAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		resp.Diagnostics.AddError("Error parsing rescue response", err.Error())
		return
	}

	plan.ServerIP = stringOrNull(apiResp.Rescue.ServerIP)
	plan.ServerIPv6Net = types.StringValue(apiResp.Rescue.ServerIPv6Net)
	plan.Active = types.BoolValue(apiResp.Rescue.Active)

	if apiResp.Rescue.Password != nil {
		plan.Password = types.StringValue(*apiResp.Rescue.Password)
	} else {
		plan.Password = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bootRescueResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state bootRescueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverNum := state.ServerNumber.ValueInt64()
	body, err := r.client.Get(fmt.Sprintf("/boot/%d/rescue", serverNum))
	if err != nil {
		resp.Diagnostics.AddError("Error reading rescue system", err.Error())
		return
	}

	var apiResp rescueAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		resp.Diagnostics.AddError("Error parsing rescue response", err.Error())
		return
	}

	state.ServerIP = stringOrNull(apiResp.Rescue.ServerIP)
	state.ServerIPv6Net = types.StringValue(apiResp.Rescue.ServerIPv6Net)
	state.Active = types.BoolValue(apiResp.Rescue.Active)

	// If rescue is active, the OS field is a string; otherwise it's an array
	if apiResp.Rescue.Active {
		if osStr, ok := apiResp.Rescue.OS.(string); ok {
			state.OS = types.StringValue(osStr)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *bootRescueResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan bootRescueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverNum := plan.ServerNumber.ValueInt64()
	data, diags := rescueForm(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Deactivate first
	_, err := r.client.Delete(fmt.Sprintf("/boot/%d/rescue", serverNum))
	if err != nil {
		resp.Diagnostics.AddError("Error deactivating rescue system", err.Error())
		return
	}

	// Reactivate with new settings

	body, err := r.client.Post(fmt.Sprintf("/boot/%d/rescue", serverNum), data)
	if err != nil {
		resp.Diagnostics.AddError("Error reactivating rescue system", err.Error())
		return
	}

	var apiResp rescueAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		resp.Diagnostics.AddError("Error parsing rescue response", err.Error())
		return
	}

	plan.ServerIP = stringOrNull(apiResp.Rescue.ServerIP)
	plan.ServerIPv6Net = types.StringValue(apiResp.Rescue.ServerIPv6Net)
	plan.Active = types.BoolValue(apiResp.Rescue.Active)

	if apiResp.Rescue.Password != nil {
		plan.Password = types.StringValue(*apiResp.Rescue.Password)
	} else {
		plan.Password = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bootRescueResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state bootRescueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverNum := state.ServerNumber.ValueInt64()
	_, err := r.client.Delete(fmt.Sprintf("/boot/%d/rescue", serverNum))
	if err != nil {
		// Booting into rescue consumes the activation, so by destroy time it is
		// usually gone already; only an error that leaves it armed is a failure.
		if active, readErr := r.isActive(serverNum); readErr == nil && !active {
			return
		}
		resp.Diagnostics.AddError("Error deactivating rescue system", err.Error())
		return
	}
}

func (r *bootRescueResource) isActive(serverNum int64) (bool, error) {
	body, err := r.client.Get(fmt.Sprintf("/boot/%d/rescue", serverNum))
	if err != nil {
		return false, err
	}
	var apiResp rescueAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return false, err
	}
	return apiResp.Rescue.Active, nil
}

func (r *bootRescueResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	serverNum, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected numeric server_number")
		return
	}

	var state bootRescueResourceModel
	state.ServerNumber = types.Int64Value(serverNum)
	state.OS = types.StringNull()
	state.Arch = types.Int64Null()
	state.AuthorizedKey = types.StringNull()
	state.AuthorizedKeys = types.ListNull(types.StringType)
	state.Keyboard = types.StringNull()
	state.ServerIP = types.StringNull()
	state.ServerIPv6Net = types.StringNull()
	state.Active = types.BoolNull()
	state.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
