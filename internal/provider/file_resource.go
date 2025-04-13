// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// todo:add integration tests

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &fileResource{}
var _ resource.ResourceWithImportState = &fileResource{}

func newFileResource(p *internalProvider) resource.Resource {
	return &fileResource{
		provider: p,
	}
}

// fileResource defines the resource implementation.
type fileResource struct {
	provider *internalProvider
}

type fileResourceModel struct {
	Path    types.String `tfsdk:"path"`
	Mode    types.String `tfsdk:"mode"`
	Owner   types.Int64  `tfsdk:"owner"`
	Group   types.Int64  `tfsdk:"group"`
	Content types.String `tfsdk:"content"`
}

func (file *fileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_file"
}

func (file *fileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "file resource",

		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:    true,
				Description: "The path of the file",
			},
			"mode": schema.StringAttribute{
				Required:    true,
				Description: "The mode of the file",
			},
			"owner": schema.Int64Attribute{
				Required:    true,
				Description: "The owner of the file",
			},
			"group": schema.Int64Attribute{
				Required:    true,
				Description: "The group of the file",
			},
			"content": schema.StringAttribute{
				Required:    true,
				Description: "The content of the file",
			},
		},
	}
}

func (file *fileResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {

}

func (file *fileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan fileResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	err := file.provider.machineAccessClient.WriteFile(ctx, plan.Path.String(), plan.Mode.String(), plan.Owner.String(), plan.Group.String(), strings.TrimPrefix(strings.TrimSuffix(plan.Content.String(), "\""), "\""))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create file", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (file *fileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model fileResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// read the file content
	content, err := file.provider.machineAccessClient.RunCommand(ctx, "sudo cat "+model.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read file", err.Error())
		return
	}

	model.Content = types.StringValue(content)

	// get the file stat
	stat, err := file.provider.machineAccessClient.RunCommand(ctx, "sudo stat -c '%u %g %a' "+model.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read file stat", err.Error())
		return
	}

	statParts := strings.Split(strings.Trim(stat, "\n"), " ")
	owner, err := strconv.ParseInt(statParts[0], 10, 64)

	if err != nil {
		resp.Diagnostics.AddError("Failed to parse owner", err.Error())
		return
	}

	group, err := strconv.ParseInt(statParts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse group", err.Error())
		return
	}

	model.Owner = types.Int64Value(owner)
	model.Group = types.Int64Value(group)
	model.Mode = types.StringValue(statParts[2])

	diags = resp.State.Set(ctx, model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (file *fileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan fileResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	err := file.provider.machineAccessClient.WriteFile(ctx, plan.Path.String(), plan.Mode.String(), plan.Owner.String(), plan.Group.String(), strings.TrimPrefix(strings.TrimSuffix(plan.Content.String(), "\""), "\""))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create file", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (file *fileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model fileResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	_, err := file.provider.machineAccessClient.RunCommand(ctx, "sudo rm -rf "+model.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete file", err.Error())
		return
	}
}

func (file *fileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
