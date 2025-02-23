// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// todo:add integration tests

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GroupResource{}
var _ resource.ResourceWithImportState = &GroupResource{}

func NewGroupResource(p *internalProvider) resource.Resource {
	return &GroupResource{
		provider: p,
	}
}

// GroupResource defines the resource implementation.
type GroupResource struct {
	provider *internalProvider
}

type groupResourceModel struct {
	Name types.String `tfsdk:"name"`
	Gid  types.Int64  `tfsdk:"gid"`
}

func (group *GroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (group *GroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Group resource",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the group",
			},
			"gid": schema.Int64Attribute{
				Computed:    true,
				Description: "The group id",
			},
		},
	}
}

func (group *GroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {

}

func (group *GroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	out, err := group.provider.machineAccessClient.RunCommand(ctx, "sudo groupadd -f "+plan.Name.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create group. Err="+err.Error()+"\nout = "+string(out), err.Error())
		return
	}

	gid, err := group.getGid(ctx, plan.Name.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to get gid", err.Error())
		return
	}
	plan.Gid = types.Int64Value(gid)
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
}

func (group *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {

	var model groupResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	gid, err := group.getGid(ctx, model.Name.String())

	if err != nil {
		resp.Diagnostics.AddError("Failed to get gid  "+err.Error(), err.Error())
		return
	}

	if gid != model.Gid.ValueInt64() {
		model.Gid = types.Int64Value(gid)
		diags = resp.State.Set(ctx, model)
		resp.Diagnostics.Append(diags...)
		if diags.HasError() {
			return
		}
	}

}

func (group *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// todo: implement me
}

func (group *GroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {

	var model groupResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	_, err := group.provider.machineAccessClient.RunCommand(ctx, "sudo groupdel "+model.Name.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete group", err.Error())
		return
	}
}

func (group *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (group *GroupResource) getGid(ctx context.Context, name string) (int64, error) {

	out, err := group.provider.machineAccessClient.RunCommand(ctx, "getent group")
	if err != nil {
		return 0, fmt.Errorf("failed to get passwd file: %w.\n out= %s", err, out)
	}
	name = strings.Replace(name, "\"", "", -1)
	tflog.Debug(ctx, "name: "+name)
	for _, line := range strings.Split(string(out), "\n") {
		line_parts := strings.Split(line, ":")
		tflog.Debug(ctx, "Line: "+strings.Join(line_parts, "\t"))

		if line_parts[0] == name {
			stringId := line_parts[2]
			id, err := strconv.ParseInt(stringId, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("failed to parse gid ('%s'): %w", stringId, err)
			}
			return id, nil
		} else {
			tflog.Debug(ctx, "Line does not start with name")
		}
	}

	return 0, fmt.Errorf("group not found")
}
