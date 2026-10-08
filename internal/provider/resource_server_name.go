// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zack/terraform-provider-hetzner/internal/client"
)

var (
	_ resource.Resource                = &serverNameResource{}
	_ resource.ResourceWithConfigure   = &serverNameResource{}
	_ resource.ResourceWithImportState = &serverNameResource{}
)

type serverNameResource struct {
	client *client.Client
}

type serverNameResourceModel struct {
	ServerNumber types.Int64  `tfsdk:"server_number"`
	ServerName   types.String `tfsdk:"server_name"`
}

func NewServerNameResource() resource.Resource {
	return &serverNameResource{}
}

func (r *serverNameResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_name"
}

func (r *serverNameResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages only the name of an existing dedicated server, for servers whose order and cancellation " +
			"live outside Terraform. Destroying it removes it from state and leaves the server and its name untouched; " +
			"use `hetzner_server_order` to manage the server's lifecycle.",
		Attributes: map[string]schema.Attribute{
			"server_number": schema.Int64Attribute{
				MarkdownDescription: "The server number.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"server_name": schema.StringAttribute{
				MarkdownDescription: "The server name shown in Robot.",
				Required:            true,
			},
		},
	}
}

func (r *serverNameResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serverNameResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverNameResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name, err := r.setName(plan.ServerNumber.ValueInt64(), plan.ServerName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error setting server name", err.Error())
		return
	}
	plan.ServerName = types.StringValue(name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverNameResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverNameResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := r.client.GetCached("/server")
	if err != nil {
		resp.Diagnostics.AddError("Error reading servers", err.Error())
		return
	}

	var apiResp []serverListAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		resp.Diagnostics.AddError("Error parsing servers response", err.Error())
		return
	}

	for _, item := range apiResp {
		if int64(item.Server.ServerNumber) == state.ServerNumber.ValueInt64() {
			state.ServerName = types.StringValue(item.Server.ServerName)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *serverNameResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverNameResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name, err := r.setName(plan.ServerNumber.ValueInt64(), plan.ServerName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating server name", err.Error())
		return
	}
	plan.ServerName = types.StringValue(name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverNameResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *serverNameResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	serverNum, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected numeric server_number")
		return
	}

	state := serverNameResourceModel{
		ServerNumber: types.Int64Value(serverNum),
		ServerName:   types.StringNull(),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverNameResource) setName(serverNum int64, name string) (string, error) {
	data := url.Values{}
	data.Set("server_name", name)

	body, err := r.client.Post(fmt.Sprintf("/server/%d", serverNum), data)
	if err != nil {
		return "", err
	}
	r.client.ForgetCached("/server")

	var apiResp serverListAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("parsing server response: %w", err)
	}
	return apiResp.Server.ServerName, nil
}
