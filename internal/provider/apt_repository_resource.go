// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// todo:add integration tests

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AptRepositoryResource{}
var _ resource.ResourceWithImportState = &AptRepositoryResource{}

func NewAptRepositoryResource(p *internalProvider) resource.Resource {
	return &AptRepositoryResource{
		provider: p,
	}
}

// AptRepositoryResource defines the resource implementation.
type AptRepositoryResource struct {
	provider *internalProvider
}

type aptRepositoryResourceModel struct {
	Key  types.String `tfsdk:"key"`
	Name types.String `tfsdk:"name"`
	URL  types.String `tfsdk:"url"`
}

func (aptRepository *AptRepositoryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apt_repository"
}

func (aptRepository *AptRepositoryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Apt key resource",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Required:    true,
				Description: "The apt key to add",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the apt repository",
			},
			"url": schema.StringAttribute{
				Optional:    true,
				Description: "The url of the apt repository",
			},
		},
	}
}

func (aptRepository *AptRepositoryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// todo: implement me
}

func (aptRepository *AptRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aptRepositoryResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	// interesting resources:
	// https://docs.docker.com/engine/install/ubuntu/#install-using-the-repository
	// https://www.geeksforgeeks.org/install-and-use-docker-on-ubuntu-2204/

	// 1. Make sure that /etc/apt/keyrings/ exists
	_, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, "sudo install -d -m 0755 /etc/apt/keyrings/")
	if err != nil {
		resp.Diagnostics.AddError("Failed to create /etc/apt/keyrings/ folder", err.Error())
		return
	}

	// 2. add the key to the keyring, i.e. copy the content of the key to /etc/apt/keyrings/<name>.asc
	err = aptRepository.provider.machineAccessClient.WriteFile(ctx, "/etc/apt/keyrings/"+plan.Name.ValueString()+".asc", "0644", "root", "root", plan.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create key file", err.Error())
		return
	}

	// 3. Get the architecture of the system by running `dpkg --print-architecture`
	archResponse, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, "dpkg --print-architecture")
	if err != nil {
		resp.Diagnostics.AddError("Failed to get system architecture", err.Error())
		return
	}
	arch := strings.Replace(string(archResponse), "\n", "", -1)

	// 4. Get the flavor of the system by running `lsb_release -cs` or `. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}"`
	flavorResponse, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, `. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}"`)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get system flavor", err.Error())
		return
	}
	flavor := strings.Replace(string(flavorResponse), "\n", "", -1)

	// 5. Add the repository to /etc/apt/sources.list.d/<name>.list with the following content:
	// 	echo \
	//   "deb [arch=$arch signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
	//   $(flavor) stable" | \
	//   sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
	_, err = aptRepository.provider.machineAccessClient.RunCommand(ctx, `echo "deb [arch=`+arch+` signed-by=/etc/apt/keyrings/`+plan.Name.ValueString()+`.asc] `+plan.URL.ValueString()+` `+flavor+` stable" | sudo tee /etc/apt/sources.list.d/`+plan.Name.ValueString()+`.list > /dev/null`)
	if err != nil {
		resp.Diagnostics.AddError("Failed to add repository to sources.list.d", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
}

func (aptRepository *AptRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// todo: implement me
}

func (aptRepository *AptRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// todo: implement me
}

func (aptRepository *AptRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// todo: implement me
}

func (aptRepository *AptRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
