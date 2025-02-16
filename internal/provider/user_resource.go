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

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

func NewUserResource(p *internalPorvider) resource.Resource {
	return &UserResource{
		provider: p,
	}
}

// UserResource defines the resource implementation.
type UserResource struct {
	provider *internalPorvider
}

type userResourceModel struct {
	Name   types.String `tfsdk:"name"`
	Uid    types.Int64  `tfsdk:"uid"`
	Groups types.List   `tfsdk:"groups"`
}

func (user *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (user *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "User resource",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the user",
			},
			"uid": schema.Int64Attribute{
				Computed:    true,
				Description: "The user id",
			},
			"groups": schema.ListAttribute{
				Optional:    true,
				ElementType: types.Int64Type,
				Description: "The groups the user belongs to, queried by gid",
			},
		},
	}
}

func (user *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {

}

func (user *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	session, err := user.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	// todo: consider adding a configation for elevated actions
	bashCmd := "sudo useradd -ms /bin/bash " + plan.Name.String()
	tflog.Warn(ctx, "Creating user with command: "+bashCmd)
	out, err := session.CombinedOutput(bashCmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create user. Err="+err.Error()+"\nout = "+string(out), err.Error())
		return
	}

	uid, err := user.getUid(ctx, plan.Name.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to get uid", err.Error())
		return
	}
	plan.Uid = types.Int64Value(uid)
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if len(plan.Groups.Elements()) > 0 {
		for _, group := range plan.Groups.Elements() {
			err := user.addUserToGroup(ctx, plan.Name.String(), group.String())
			if err != nil {
				resp.Diagnostics.AddError("Failed to add user to group", err.Error())
				return
			}
		}
	}
}

func (user *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model userResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	uid, err := user.getUid(ctx, model.Name.String())

	if err != nil {
		resp.Diagnostics.AddError("Failed to get uid  "+err.Error(), err.Error())
		return
	}

	if uid != model.Uid.ValueInt64() {
		model.Uid = types.Int64Value(uid)
		diags = resp.State.Set(ctx, model)
		resp.Diagnostics.Append(diags...)
		if diags.HasError() {
			return
		}
	}

}

func (user *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// todo: implement update when a user is added to a group
}

func (user *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model userResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	session, err := user.provider.sshClient.NewSession()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ssh session", err.Error())
		return
	}
	defer session.Close()

	_, err = session.CombinedOutput("sudo userdel " + model.Name.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete user", err.Error())
		return
	}
}

func (user *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (user *UserResource) getUid(ctx context.Context, name string) (int64, error) {
	session, err := user.provider.sshClient.NewSession()
	if err != nil {
		return 0, err
	}

	out, err := session.CombinedOutput("cat /etc/passwd")
	if err != nil {
		return 0, fmt.Errorf("Failed to get passwd file: %w.\n out= %s", err, out)
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
				return 0, fmt.Errorf("Failed to parse uid ('%s'): %w", stringId, err)
			}
			return id, nil
		} else {
			tflog.Debug(ctx, "Line does not start with name")
		}
	}

	return 0, fmt.Errorf("User not found")
}

func (user *UserResource) addUserToGroup(ctx context.Context, name string, group string) error {
	session, err := user.provider.sshClient.NewSession()
	if err != nil {
		return err
	}
	command := "sudo usermod -aG " + group + " " + name
	tflog.Warn(ctx, "Adding user to group with command: "+command)
	_, err = session.CombinedOutput(command)
	if err != nil {
		return fmt.Errorf("Failed to add user to group: %w", err)
	}
	return nil
}
