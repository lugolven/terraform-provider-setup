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
var _ resource.Resource = &directoryResource{}
var _ resource.ResourceWithImportState = &directoryResource{}

func newDirectoryResource(p *internalProvider) resource.Resource {
	return &directoryResource{
		provider: p,
	}
}

// directoryResource defines the resource implementation.
type directoryResource struct {
	provider *internalProvider
}

type directoryResourceModel struct {
	Path  types.String `tfsdk:"path"`
	Mode  types.String `tfsdk:"mode"`
	Owner types.Int64  `tfsdk:"owner"`
	Group types.Int64  `tfsdk:"group"`
}

func (directory *directoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_directory"
}

func (directory *directoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Directory resource",

		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:    true,
				Description: "The path of the directory",
			},
			"mode": schema.StringAttribute{
				Required:    true,
				Description: "The mode of the directory",
			},
			"owner": schema.Int64Attribute{
				Required:    true,
				Description: "The owner of the directory",
			},
			"group": schema.Int64Attribute{
				Required:    true,
				Description: "The group of the directory",
			},
		},
	}
}

func (directory *directoryResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {

}

func (directory *directoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan directoryResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// todo: consider adding a configation for elevated actions
	out, err := directory.provider.machineAccessClient.RunCommand(ctx, "sudo install -d -m "+plan.Mode.String()+" -o "+plan.Owner.String()+" -g "+plan.Group.String()+" "+plan.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create directory. Err="+err.Error()+"\nout = "+string(out), err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (directory *directoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model directoryResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (directory *directoryResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// todo: implement update
}

func (directory *directoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model directoryResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	_, err := directory.provider.machineAccessClient.RunCommand(ctx, "sudo rm -rf "+model.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete directory", err.Error())
		return
	}
}

func (directory *directoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
