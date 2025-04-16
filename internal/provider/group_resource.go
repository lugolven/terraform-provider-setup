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
var _ resource.Resource = &groupResource{}
var _ resource.ResourceWithImportState = &groupResource{}

func newGroupResource(p *internalProvider) resource.Resource {
	return &groupResource{
		provider: p,
	}
}

// groupResource defines the resource implementation.
type groupResource struct {
	provider *internalProvider
}

type groupResourceModel struct {
	Name types.String `tfsdk:"name"`
	Gid  types.Int64  `tfsdk:"gid"`
}

func (group *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (group *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (group *groupResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {

}

func (group *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
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

	gid, err := group.getGid(ctx, plan.Name)
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

func (group *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model groupResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	gid, err := group.getGid(ctx, model.Name)

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

func (group *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var oldModel groupResourceModel
	diags := req.State.Get(ctx, &oldModel)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	var newModel groupResourceModel
	diags = req.Plan.Get(ctx, &newModel)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	if oldModel.Name.String() != newModel.Name.String() {
		_, err := group.provider.machineAccessClient.RunCommand(ctx, "sudo groupmod -n "+newModel.Name.String()+" "+oldModel.Name.String())
		if err != nil {
			resp.Diagnostics.AddError("Failed to update group", err.Error())
			return
		}
	}

	gid, err := group.getGid(ctx, newModel.Name)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get gid", err.Error())
		return
	}

	newModel.Gid = types.Int64Value(gid)
	diags = resp.State.Set(ctx, newModel)
	resp.Diagnostics.Append(diags...)
}

func (group *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
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

func (group *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (group *groupResource) getGid(ctx context.Context, inputName types.String) (int64, error) {
	out, err := group.provider.machineAccessClient.RunCommand(ctx, "getent group")
	if err != nil {
		return 0, fmt.Errorf("failed to get passwd file: %w.\n out= %s", err, out)
	}

	name, err := strconv.Unquote(inputName.String())
	if err != nil {
		return 0, fmt.Errorf("failed to unquote name: %w", err)
	}

	tflog.Debug(ctx, "name: "+name)

	for _, line := range strings.Split(string(out), "\n") {
		lineParts := strings.Split(line, ":")
		tflog.Debug(ctx, "Line: "+strings.Join(lineParts, "\t"))

		if lineParts[0] == name {
			stringID := lineParts[2]
			id, err := strconv.ParseInt(stringID, 10, 64)

			if err != nil {
				return 0, fmt.Errorf("failed to parse gid ('%s'): %w", stringID, err)
			}

			return id, nil
		}

		tflog.Debug(ctx, "Line does not start with name")
	}

	return 0, fmt.Errorf("group not found")
}
