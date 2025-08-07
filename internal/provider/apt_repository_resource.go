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
var _ resource.Resource = &aptRepositoryResource{}
var _ resource.ResourceWithImportState = &aptRepositoryResource{}

func newAptRepositoryResource(p *internalProvider) resource.Resource {
	return &aptRepositoryResource{
		provider: p,
	}
}

// aptRepositoryResource defines the resource implementation.
type aptRepositoryResource struct {
	provider *internalProvider
}

type aptRepositoryResourceModel struct {
	Key  types.String `tfsdk:"key"`
	Name types.String `tfsdk:"name"`
	URL  types.String `tfsdk:"url"`
}

func (aptRepository *aptRepositoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apt_repository"
}

func (aptRepository *aptRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (aptRepository *aptRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	provider, ok := req.ProviderData.(*internalProvider)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			"Expected *internalProvider, got: %T. Please report this issue to the provider developers.",
		)

		return
	}

	aptRepository.provider = provider
}

func (aptRepository *aptRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
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

	// 6. Update apt package cache to ensure the repository is accessible
	updateOutput, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, "sudo apt update")
	if err != nil {
		resp.Diagnostics.AddError("Failed to update apt package cache after adding repository", "This usually means the repository URL is invalid, the GPG key is incorrect, or the repository doesn't support your system architecture/distribution.\n\nRepository: "+plan.URL.ValueString()+" "+flavor+"\nArchitecture: "+arch+"\n\nError: "+err.Error()+"\n\nOutput: "+string(updateOutput))
		return
	}

	// 7. Check for any repository-related errors in the update output
	updateOutputStr := string(updateOutput)
	if strings.Contains(updateOutputStr, "Failed to fetch") || strings.Contains(updateOutputStr, "NO_PUBKEY") || strings.Contains(updateOutputStr, "KEYEXPIRED") {
		resp.Diagnostics.AddError("Repository validation failed", "The repository was added but apt update shows errors:\n\n"+updateOutputStr)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (aptRepository *aptRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aptRepositoryResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Check if the repository key file exists
	keyPath := "/etc/apt/keyrings/" + state.Name.ValueString() + ".asc"
	_, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, "test -f "+keyPath)

	if err != nil {
		// Key file doesn't exist, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// Check if the repository source list exists
	sourceListPath := "/etc/apt/sources.list.d/" + state.Name.ValueString() + ".list"
	_, err = aptRepository.provider.machineAccessClient.RunCommand(ctx, "test -f "+sourceListPath)

	if err != nil {
		// Source list doesn't exist, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// Repository exists, keep the current state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (aptRepository *aptRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan aptRepositoryResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	var state aptRepositoryResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// If the name changed, we need to delete the old files and create new ones
	if !plan.Name.Equal(state.Name) {
		// Remove old key file
		oldKeyPath := "/etc/apt/keyrings/" + state.Name.ValueString() + ".asc"
		_, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, "sudo rm -f "+oldKeyPath)

		if err != nil {
			resp.Diagnostics.AddWarning("Failed to remove old key file", err.Error())
		}

		// Remove old source list
		oldSourceListPath := "/etc/apt/sources.list.d/" + state.Name.ValueString() + ".list"
		_, err = aptRepository.provider.machineAccessClient.RunCommand(ctx, "sudo rm -f "+oldSourceListPath)

		if err != nil {
			resp.Diagnostics.AddWarning("Failed to remove old source list", err.Error())
		}
	}

	// Update the key file
	err := aptRepository.provider.machineAccessClient.WriteFile(ctx, "/etc/apt/keyrings/"+plan.Name.ValueString()+".asc", "0644", "root", "root", plan.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update key file", err.Error())
		return
	}

	// Get system architecture and flavor
	archResponse, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, "dpkg --print-architecture")
	if err != nil {
		resp.Diagnostics.AddError("Failed to get system architecture", err.Error())
		return
	}

	arch := strings.Replace(string(archResponse), "\n", "", -1)

	flavorResponse, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, `. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}"`)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get system flavor", err.Error())
		return
	}

	flavor := strings.Replace(string(flavorResponse), "\n", "", -1)

	// Update the repository source list
	_, err = aptRepository.provider.machineAccessClient.RunCommand(ctx, `echo "deb [arch=`+arch+` signed-by=/etc/apt/keyrings/`+plan.Name.ValueString()+`.asc] `+plan.URL.ValueString()+` `+flavor+` stable" | sudo tee /etc/apt/sources.list.d/`+plan.Name.ValueString()+`.list > /dev/null`)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update repository source list", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (aptRepository *aptRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aptRepositoryResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Remove the key file
	keyPath := "/etc/apt/keyrings/" + state.Name.ValueString() + ".asc"
	_, err := aptRepository.provider.machineAccessClient.RunCommand(ctx, "sudo rm -f "+keyPath)

	if err != nil {
		resp.Diagnostics.AddWarning("Failed to remove key file", err.Error())
	}

	// Remove the source list file
	sourceListPath := "/etc/apt/sources.list.d/" + state.Name.ValueString() + ".list"
	_, err = aptRepository.provider.machineAccessClient.RunCommand(ctx, "sudo rm -f "+sourceListPath)

	if err != nil {
		resp.Diagnostics.AddWarning("Failed to remove source list file", err.Error())
	}

	// Update apt package cache to reflect the changes
	_, err = aptRepository.provider.machineAccessClient.RunCommand(ctx, "sudo apt-get update")
	if err != nil {
		resp.Diagnostics.AddWarning("Failed to update apt package cache", err.Error())
	}
}

func (aptRepository *aptRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
