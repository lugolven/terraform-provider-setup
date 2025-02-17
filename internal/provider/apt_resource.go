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
var _ resource.Resource = &AptResource{}
var _ resource.ResourceWithImportState = &AptResource{}

func NewAptResource(p *internalProvider) resource.Resource {
	return &AptResource{
		provider: p,
	}
}

// AptResource defines the resource implementation.
type AptResource struct {
	provider *internalProvider
}

type aptResourceModel struct {
	Removed types.List `tfsdk:"removed"`
}

func (apt *AptResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apt"
}

func (apt *AptResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Apt resource",
		Attributes: map[string]schema.Attribute{
			"removed": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The list of apt package to remove",
			},
		},
	}
}

func (apt *AptResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {

}

func (apt *AptResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aptResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	session, err := apt.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	elements_to_remove := plan.Removed.Elements()
	cmd := "sudo apt-get remove -y "

	for _, element := range elements_to_remove {
		cmd += element.String() + " "
	}

	tflog.Debug(ctx, "Removing apt packages with command: "+cmd)
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to remove apt packages. Err="+err.Error()+"\nout = "+string(out), err.Error())
		return
	}

	session, err = apt.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	cmd = "sudo apt autoremove -y"
	tflog.Debug(ctx, "Auto-removing apt packages with command: "+cmd)

	out, err = session.CombinedOutput(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to auto-remove apt packages. Err="+err.Error()+"\nout = "+string(out), err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
}

func (apt *AptResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// todo
}

func (apt *AptResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// todo: implement update when a user is added to a group
}

func (apt *AptResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	return
}

func (apt *AptResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
