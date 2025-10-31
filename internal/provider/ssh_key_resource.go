// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &sshKeyResource{}
var _ resource.ResourceWithImportState = &sshKeyResource{}

func newSSHKeyResource(p *internalProvider) resource.Resource {
	return &sshKeyResource{
		provider: p,
	}
}

// sshKeyResource defines the resource implementation.
type sshKeyResource struct {
	provider *internalProvider
}

const (
	keyTypeRSA = "rsa"
	keyTypeDSA = "dsa"
)

type sshKeyResourceModel struct {
	Path      types.String `tfsdk:"path"`
	KeyType   types.String `tfsdk:"key_type"`
	KeySize   types.Int64  `tfsdk:"key_size"`
	PublicKey types.String `tfsdk:"public_key"`
	Owner     types.String `tfsdk:"owner"`
	Group     types.String `tfsdk:"group"`
	Mode      types.String `tfsdk:"mode"`
}

func (r *sshKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (r *sshKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "SSH Key resource that generates SSH keys using ssh-keygen on the remote machine",

		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:    true,
				Description: "The path where the SSH private key will be stored (public key will be stored at path.pub)",
			},
			"key_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The type of SSH key to generate (rsa, ed25519, ecdsa, dsa). Defaults to 'rsa'",
			},
			"key_size": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The size of the SSH key in bits. Defaults to 2048 for RSA keys",
			},
			"public_key": schema.StringAttribute{
				Computed:    true,
				Description: "The generated public key content",
			},
			"owner": schema.StringAttribute{
				Optional:    true,
				Description: "The owner (user) of the SSH key files. If not specified, the current user is used",
			},
			"group": schema.StringAttribute{
				Optional:    true,
				Description: "The group of the SSH key files. If not specified, the current group is used",
			},
			"mode": schema.StringAttribute{
				Optional:    true,
				Description: "The permissions of the SSH key files in octal format (e.g., '0600'). If not specified, defaults to '0600' for private key and '0644' for public key",
			},
		},
	}
}

func (r *sshKeyResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {

}

func (r *sshKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sshKeyResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Set defaults
	keyType := keyTypeRSA
	if !plan.KeyType.IsNull() && !plan.KeyType.IsUnknown() {
		keyType = plan.KeyType.ValueString()
	}

	keySize := int64(2048)
	if !plan.KeySize.IsNull() && !plan.KeySize.IsUnknown() {
		keySize = plan.KeySize.ValueInt64()
	}

	// Build ssh-keygen command
	var cmd strings.Builder

	cmd.WriteString("ssh-keygen -t ")
	cmd.WriteString(keyType)

	// Only add key size for RSA and DSA keys
	if keyType == keyTypeRSA || keyType == keyTypeDSA {
		cmd.WriteString(fmt.Sprintf(" -b %d", keySize))
	}

	cmd.WriteString(" -f ")
	cmd.WriteString(plan.Path.ValueString())
	cmd.WriteString(" -N ''") // No passphrase

	// Generate the SSH key
	_, err := r.provider.machineAccessClient.RunCommand(ctx, cmd.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to generate SSH key", err.Error())
		return
	}

	// Read the public key
	publicKeyPath := plan.Path.ValueString() + ".pub"

	publicKeyContent, err := r.provider.machineAccessClient.RunCommand(ctx, "cat "+publicKeyPath)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read public key", err.Error())
		return
	}

	// Set owner and group if specified
	if !plan.Owner.IsNull() && !plan.Owner.IsUnknown() {
		ownerStr := plan.Owner.ValueString()

		groupStr := ""
		if !plan.Group.IsNull() && !plan.Group.IsUnknown() {
			groupStr = plan.Group.ValueString()
		}

		// Build chown command with sudo
		var chownCmd strings.Builder
		chownCmd.WriteString("sudo chown ")
		chownCmd.WriteString(ownerStr)

		if groupStr != "" {
			chownCmd.WriteString(":")
			chownCmd.WriteString(groupStr)
		}

		chownCmd.WriteString(" ")
		chownCmd.WriteString(plan.Path.ValueString())
		chownCmd.WriteString(" ")
		chownCmd.WriteString(publicKeyPath)

		_, err := r.provider.machineAccessClient.RunCommand(ctx, chownCmd.String())
		if err != nil {
			resp.Diagnostics.AddError("Failed to set owner and group", err.Error())
			return
		}
	}

	// Set file mode if specified
	if !plan.Mode.IsNull() && !plan.Mode.IsUnknown() {
		modeStr := plan.Mode.ValueString()

		// Build chmod command with sudo
		var chmodCmd strings.Builder
		chmodCmd.WriteString("sudo chmod ")
		chmodCmd.WriteString(modeStr)
		chmodCmd.WriteString(" ")
		chmodCmd.WriteString(plan.Path.ValueString())
		chmodCmd.WriteString(" ")
		chmodCmd.WriteString(publicKeyPath)

		_, err := r.provider.machineAccessClient.RunCommand(ctx, chmodCmd.String())
		if err != nil {
			resp.Diagnostics.AddError("Failed to set file mode", err.Error())
			return
		}
	}

	// Update the model with computed values
	plan.KeyType = types.StringValue(keyType)
	plan.KeySize = types.Int64Value(keySize)
	plan.PublicKey = types.StringValue(strings.TrimSpace(publicKeyContent))

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (r *sshKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model sshKeyResourceModel

	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Check if private key exists
	_, err := r.provider.machineAccessClient.RunCommand(ctx, "test -f "+model.Path.ValueString())
	if err != nil {
		// If private key doesn't exist, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// Read the public key
	publicKeyPath := model.Path.ValueString() + ".pub"

	publicKeyContent, err := r.provider.machineAccessClient.RunCommand(ctx, "cat "+publicKeyPath)
	if err != nil {
		// If public key doesn't exist, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	model.PublicKey = types.StringValue(strings.TrimSpace(publicKeyContent))

	diags = resp.State.Set(ctx, model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (r *sshKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sshKeyResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	var state sshKeyResourceModel

	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// If path, key_type, or key_size changed, we need to regenerate the key
	if !plan.Path.Equal(state.Path) || !plan.KeyType.Equal(state.KeyType) || !plan.KeySize.Equal(state.KeySize) {
		// Delete old keys first (use sudo if owner was set)
		var deleteCmd string
		if !state.Owner.IsNull() && !state.Owner.IsUnknown() {
			deleteCmd = "sudo rm -f " + state.Path.ValueString() + " " + state.Path.ValueString() + ".pub"
		} else {
			deleteCmd = "rm -f " + state.Path.ValueString() + " " + state.Path.ValueString() + ".pub"
		}

		_, _ = r.provider.machineAccessClient.RunCommand(ctx, deleteCmd)

		// Set defaults for new key
		keyType := keyTypeRSA
		if !plan.KeyType.IsNull() && !plan.KeyType.IsUnknown() {
			keyType = plan.KeyType.ValueString()
		}

		keySize := int64(2048)
		if !plan.KeySize.IsNull() && !plan.KeySize.IsUnknown() {
			keySize = plan.KeySize.ValueInt64()
		}

		// Build ssh-keygen command
		var cmd strings.Builder

		cmd.WriteString("ssh-keygen -t ")
		cmd.WriteString(keyType)

		// Only add key size for RSA and DSA keys
		if keyType == keyTypeRSA || keyType == keyTypeDSA {
			cmd.WriteString(fmt.Sprintf(" -b %d", keySize))
		}

		cmd.WriteString(" -f ")
		cmd.WriteString(plan.Path.ValueString())
		cmd.WriteString(" -N ''") // No passphrase

		// Generate the SSH key
		_, err := r.provider.machineAccessClient.RunCommand(ctx, cmd.String())
		if err != nil {
			resp.Diagnostics.AddError("Failed to generate SSH key", err.Error())
			return
		}

		// Read the public key
		publicKeyPath := plan.Path.ValueString() + ".pub"

		publicKeyContent, err := r.provider.machineAccessClient.RunCommand(ctx, "cat "+publicKeyPath)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read public key", err.Error())
			return
		}

		// Update the model with computed values
		plan.KeyType = types.StringValue(keyType)
		plan.KeySize = types.Int64Value(keySize)
		plan.PublicKey = types.StringValue(strings.TrimSpace(publicKeyContent))
	}

	// If owner or group changed, update the ownership
	if !plan.Owner.Equal(state.Owner) || !plan.Group.Equal(state.Group) {
		if !plan.Owner.IsNull() && !plan.Owner.IsUnknown() {
			ownerStr := plan.Owner.ValueString()

			groupStr := ""
			if !plan.Group.IsNull() && !plan.Group.IsUnknown() {
				groupStr = plan.Group.ValueString()
			}

			// Build chown command with sudo
			var chownCmd strings.Builder
			chownCmd.WriteString("sudo chown ")
			chownCmd.WriteString(ownerStr)

			if groupStr != "" {
				chownCmd.WriteString(":")
				chownCmd.WriteString(groupStr)
			}

			chownCmd.WriteString(" ")
			chownCmd.WriteString(plan.Path.ValueString())
			chownCmd.WriteString(" ")
			chownCmd.WriteString(plan.Path.ValueString() + ".pub")

			_, err := r.provider.machineAccessClient.RunCommand(ctx, chownCmd.String())
			if err != nil {
				resp.Diagnostics.AddError("Failed to set owner and group", err.Error())
				return
			}
		}
	}

	// If mode changed, update the file permissions
	if !plan.Mode.Equal(state.Mode) {
		if !plan.Mode.IsNull() && !plan.Mode.IsUnknown() {
			modeStr := plan.Mode.ValueString()

			// Build chmod command with sudo
			var chmodCmd strings.Builder
			chmodCmd.WriteString("sudo chmod ")
			chmodCmd.WriteString(modeStr)
			chmodCmd.WriteString(" ")
			chmodCmd.WriteString(plan.Path.ValueString())
			chmodCmd.WriteString(" ")
			chmodCmd.WriteString(plan.Path.ValueString() + ".pub")

			_, err := r.provider.machineAccessClient.RunCommand(ctx, chmodCmd.String())
			if err != nil {
				resp.Diagnostics.AddError("Failed to set file mode", err.Error())
				return
			}
		}
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (r *sshKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model sshKeyResourceModel

	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Delete both private and public key files
	// Use sudo if owner is not the current user
	var deleteCmd string
	if !model.Owner.IsNull() && !model.Owner.IsUnknown() {
		deleteCmd = "sudo rm -f " + model.Path.ValueString() + " " + model.Path.ValueString() + ".pub"
	} else {
		deleteCmd = "rm -f " + model.Path.ValueString() + " " + model.Path.ValueString() + ".pub"
	}

	_, err := r.provider.machineAccessClient.RunCommand(ctx, deleteCmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete SSH key", err.Error())
		return
	}
}

func (r *sshKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("path"), req, resp)
}
