// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// todo:add integration tests

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &FileResource{}
var _ resource.ResourceWithImportState = &FileResource{}

func NewFileResource(p *internalProvider) resource.Resource {
	return &FileResource{
		provider: p,
	}
}

// FileResource defines the resource implementation.
type FileResource struct {
	provider *internalProvider
}

type fileResourceModel struct {
	Path    types.String `tfsdk:"path"`
	Mode    types.String `tfsdk:"mode"`
	Owner   types.Int64  `tfsdk:"owner"`
	Group   types.Int64  `tfsdk:"group"`
	Content types.String `tfsdk:"content"`
}

func (file *FileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_file"
}

func (file *FileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (file *FileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {

}

func (file *FileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan fileResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	err := file.provider.machineAccessClient.WriteFile(ctx, plan.Path.String(), plan.Mode.String(), plan.Owner.String(), plan.Group.String(), plan.Content.String())
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

func (file *FileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model fileResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
}

func (file *FileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// todo: implement update
}

func (file *FileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
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

func (file *FileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
