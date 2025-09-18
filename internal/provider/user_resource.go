// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"terraform-provider-setup/internal/provider/clients"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &userResource{}
var _ resource.ResourceWithImportState = &userResource{}

func newUserResource(p *internalProvider) resource.Resource {
	return &userResource{
		provider: p,
	}
}

// userResource defines the resource implementation.
type userResource struct {
	provider *internalProvider
}

type userResourceModel struct {
	Name   types.String `tfsdk:"name"`
	UID    types.Int64  `tfsdk:"uid"`
	Groups types.List   `tfsdk:"groups"`
}

func (user *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (user *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (user *userResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	// todo: implement me
}

func (user *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// todo: consider adding a configation for elevated actions
	out, err := user.provider.machineAccessClient.RunCommand(ctx, "sudo useradd -ms /bin/bash "+plan.Name.String())
	if err != nil {
		if exitErr, ok := err.(clients.ExitError); ok && exitErr.ExitCode == 9 {
			tflog.Debug(ctx, "User already exists")
		} else {
			resp.Diagnostics.AddError("Failed to create user. Err="+err.Error()+"\nout = "+string(out), err.Error())
			return
		}
	}

	uid, err := user.getUID(ctx, plan.Name)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get uid", err.Error())
		return
	}

	plan.UID = types.Int64Value(uid)
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

func (user *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model userResourceModel

	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	uid, err := user.getUID(ctx, model.Name)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get uid  "+err.Error(), err.Error())
		return
	}

	if uid != model.UID.ValueInt64() {
		model.UID = types.Int64Value(uid)
		diags = resp.State.Set(ctx, model)
		resp.Diagnostics.Append(diags...)

		if diags.HasError() {
			return
		}
	}
}

func (user *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var oldModel userResourceModel

	diags := req.State.Get(ctx, &oldModel)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	var newModel userResourceModel

	diags = req.Plan.Get(ctx, &newModel)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	if oldModel.Name != newModel.Name {
		_, err := user.provider.machineAccessClient.RunCommand(ctx, "sudo usermod -l "+newModel.Name.String()+" "+oldModel.Name.String())
		if err != nil {
			resp.Diagnostics.AddError("Failed to update user", err.Error())
			return
		}
	}

	// Remove groups that are not in the new model
	for _, group := range oldModel.Groups.Elements() {
		found := false

		for _, newGroup := range newModel.Groups.Elements() {
			if group.Equal(newGroup) {
				found = true
				break
			}
		}

		if !found {
			groupGid, err := strconv.ParseInt(group.String(), 10, 64)
			if err != nil {
				resp.Diagnostics.AddError("Failed to parse group ID", err.Error())
				return
			}

			groupName, err := user.getGroupNameFromGid(ctx, groupGid)
			if err != nil {
				tflog.Debug(ctx, "Group with gid does not exist, skipping removal: "+group.String())
				continue
			}

			_, err = user.provider.machineAccessClient.RunCommand(ctx, "sudo deluser "+oldModel.Name.String()+" "+groupName)
			if err != nil {
				resp.Diagnostics.AddError("Failed to remove user from group", err.Error())
				return
			}
		}
	}

	// Add groups that are in the new model but not in the old model
	for _, newGroup := range newModel.Groups.Elements() {
		found := false

		for _, group := range oldModel.Groups.Elements() {
			if group.Equal(newGroup) {
				found = true
				break
			}
		}

		if !found {
			err := user.addUserToGroup(ctx, newModel.Name.String(), newGroup.String())
			if err != nil {
				resp.Diagnostics.AddError("Failed to add user to group", err.Error())
				return
			}
		}
	}

	uid, err := user.getUID(ctx, newModel.Name)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get uid", err.Error())
		return
	}

	newModel.UID = types.Int64Value(uid)
	diags = resp.State.Set(ctx, newModel)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (user *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model userResourceModel

	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	_, err := user.provider.machineAccessClient.RunCommand(ctx, "sudo userdel "+model.Name.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete user", err.Error())
		return
	}
}

func (user *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (user *userResource) getUID(ctx context.Context, inputName types.String) (int64, error) {
	out, err := user.provider.machineAccessClient.RunCommand(ctx, "cat /etc/passwd")
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
				return 0, fmt.Errorf("failed to parse uid ('%s'): %w", stringID, err)
			}

			return id, nil
		}

		tflog.Debug(ctx, "Line does not start with name")
	}

	return 0, fmt.Errorf("user not found")
}

func (user *userResource) addUserToGroup(ctx context.Context, name string, group string) error {
	_, err := user.provider.machineAccessClient.RunCommand(ctx, "sudo usermod -aG "+group+" "+name)
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w", err)
	}

	return nil
}

func (user *userResource) getGroupNameFromGid(ctx context.Context, gid int64) (string, error) {
	out, err := user.provider.machineAccessClient.RunCommand(ctx, "getent group")
	if err != nil {
		return "", fmt.Errorf("failed to get group file: %w.\n out= %s", err, out)
	}

	for _, line := range strings.Split(string(out), "\n") {
		lineParts := strings.Split(line, ":")
		if len(lineParts) < 3 {
			continue
		}

		groupGid, err := strconv.ParseInt(lineParts[2], 10, 64)
		if err != nil {
			continue
		}

		if groupGid == gid {
			return lineParts[0], nil
		}
	}

	return "", fmt.Errorf("group with gid %d not found", gid)
}
