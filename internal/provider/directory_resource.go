// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &DirectoryResource{}
var _ resource.ResourceWithImportState = &DirectoryResource{}

func NewDirectoryResource(p *internalPorvider) resource.Resource {
	return &DirectoryResource{
		provider: p,
	}
}

// DirectoryResource defines the resource implementation.
type DirectoryResource struct {
	provider *internalPorvider
}

type directoryResourceModel struct {
	Path  types.String `tfsdk:"path"`
	Mode  types.String `tfsdk:"mode"`
	Owner types.Int64  `tfsdk:"owner"`
	Group types.Int64  `tfsdk:"group"`
}

func (directory *DirectoryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_directory"
}

func (directory *DirectoryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (directory *DirectoryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {

}

func (directory *DirectoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan directoryResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	session, err := directory.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	// todo: consider adding a configation for elevated actions
	bashCmd := "sudo install -d -m " + plan.Mode.String() + " -o " + plan.Owner.String() + " -g " + plan.Group.String() + " " + plan.Path.String()
	tflog.Warn(ctx, "Creating directory with command: "+bashCmd)
	out, err := session.CombinedOutput(bashCmd)
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

func (directory *DirectoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model directoryResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
}

func (directory *DirectoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// todo: implement update
}

func (directory *DirectoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model directoryResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	session, err := directory.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	_, err = session.CombinedOutput("sudo rm -rf " + model.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete directory", err.Error())
		return
	}
}

func (directory *DirectoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
