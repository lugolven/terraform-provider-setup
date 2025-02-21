// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
	session, err := file.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	scpClient, err := scp.NewClientBySSH(file.provider.sshClient)
	if err != nil {
		fmt.Println("Error creating new SSH session from existing connection", err)
	}

	// write file content to tmp file from the host
	tflog.Debug(ctx, "Writing file content to temp file")
	tmpFile, err := os.CreateTemp("", "tempfile")
	if err != nil {
		resp.Diagnostics.AddError("Failed to create temp file", err.Error())
		return
	}
	defer os.Remove(tmpFile.Name())

	tflog.Debug(ctx, "Writing content '"+plan.Content.String()+"'to temp file "+tmpFile.Name())
	err = os.WriteFile(tmpFile.Name(), []byte(plan.Content.String()), 0755)
	if err != nil {
		resp.Diagnostics.AddError("Failed to write to temp file", err.Error())
		return
	}

	tflog.Debug(ctx, "Copying file to remote host "+plan.Path.String())

	// copy the file to the remote host
	f, _ := os.Open(tmpFile.Name())
	remoteTmpFile, _ := os.CreateTemp("", "tempfile")
	err = scpClient.CopyFromFile(ctx, *f, remoteTmpFile.Name(), "0700")
	if err != nil {
		resp.Diagnostics.AddError("Failed to copy file to remote host", err.Error())
		return
	}

	// move the file to the correct location
	_, err = session.CombinedOutput("sudo mv " + remoteTmpFile.Name() + " " + plan.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to move file to correct location", err.Error())
		return
	}

	// set the owner and group of the remote file

	bashCmd := "sudo chown " + plan.Owner.String() + ":" + plan.Group.String() + " " + plan.Path.String()
	tflog.Warn(ctx, "Setting file owner and group with command: "+bashCmd)
	session, err = file.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()
	out, err := session.CombinedOutput(bashCmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to set file owner and group. Err="+err.Error()+"\nout = "+string(out), err.Error())
		return
	}

	session, err = file.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	// set the mode of the remote file
	bashCmd = "sudo chmod " + plan.Mode.String() + " " + plan.Path.String()
	tflog.Warn(ctx, "Setting file mode with command: "+bashCmd)

	out, err = session.CombinedOutput(bashCmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to set file mode. Err="+err.Error()+"\nout = "+string(out), err.Error())
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

	session, err := file.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	_, err = session.CombinedOutput("sudo rm -rf " + model.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete file", err.Error())
		return
	}
}

func (file *FileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
