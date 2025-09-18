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
var _ resource.Resource = &sshAddResource{}
var _ resource.ResourceWithImportState = &sshAddResource{}

func newSSHAddResource(p *internalProvider) resource.Resource {
	return &sshAddResource{
		provider: p,
	}
}

// sshAddResource defines the resource implementation.
type sshAddResource struct {
	provider *internalProvider
}

type sshAddResourceModel struct {
	AuthorizedKeysPath types.String `tfsdk:"authorized_keys_path"`
	PublicKey          types.String `tfsdk:"public_key"`
	Comment            types.String `tfsdk:"comment"`
	ID                 types.String `tfsdk:"id"`
}

func (r *sshAddResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_add"
}

func (r *sshAddResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "SSH Add resource that adds a public key to an authorized_keys file with an optional comment",

		Attributes: map[string]schema.Attribute{
			"authorized_keys_path": schema.StringAttribute{
				Required:    true,
				Description: "The path to the authorized_keys file",
			},
			"public_key": schema.StringAttribute{
				Required:    true,
				Description: "The public key content to add",
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Description: "An optional comment to append to the public key",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for this resource",
			},
		},
	}
}

func (r *sshAddResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {

}

func (r *sshAddResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sshAddResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Prepare the key entry
	publicKey := strings.TrimSpace(plan.PublicKey.ValueString())
	keyEntry := publicKey

	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() && plan.Comment.ValueString() != "" {
		keyEntry = fmt.Sprintf("%s %s", publicKey, plan.Comment.ValueString())
	}

	// Ensure the authorized_keys directory exists
	authorizedKeysDir := plan.AuthorizedKeysPath.ValueString()
	if strings.Contains(authorizedKeysDir, "/") {
		dirPath := authorizedKeysDir[:strings.LastIndex(authorizedKeysDir, "/")]

		_, err := r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("mkdir -p %s", dirPath))
		if err != nil {
			resp.Diagnostics.AddError("Failed to create authorized_keys directory", err.Error())
			return
		}
	}

	// Check if the key already exists in the file
	content, err := r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("cat %s 2>/dev/null || echo ''", plan.AuthorizedKeysPath.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read authorized_keys file", err.Error())
		return
	}

	// Check if the public key already exists (ignoring comments)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Extract just the key part (first two fields typically: key-type and key-data)
		parts := strings.Fields(line)
		existingParts := strings.Fields(publicKey)

		if len(parts) >= 2 && len(existingParts) >= 2 {
			if parts[0] == existingParts[0] && parts[1] == existingParts[1] {
				resp.Diagnostics.AddError("Public key already exists", "The public key is already present in the authorized_keys file")
				return
			}
		}
	}

	// Append the key to the authorized_keys file
	_, err = r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("echo '%s' >> %s", keyEntry, plan.AuthorizedKeysPath.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to add key to authorized_keys", err.Error())
		return
	}

	// Set appropriate permissions on the authorized_keys file
	_, err = r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("chmod 600 %s", plan.AuthorizedKeysPath.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to set permissions on authorized_keys", err.Error())
		return
	}

	// Generate a unique ID based on the key and path
	plan.ID = types.StringValue(fmt.Sprintf("%s:%s", plan.AuthorizedKeysPath.ValueString(), publicKey))

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (r *sshAddResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model sshAddResourceModel

	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Read the authorized_keys file
	content, err := r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("cat %s 2>/dev/null || echo ''", model.AuthorizedKeysPath.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to read authorized_keys file", err.Error())
		return
	}

	// Check if our public key still exists
	publicKey := strings.TrimSpace(model.PublicKey.ValueString())
	lines := strings.Split(content, "\n")
	keyExists := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Extract just the key part (first two fields typically: key-type and key-data)
		parts := strings.Fields(line)
		existingParts := strings.Fields(publicKey)

		if len(parts) >= 2 && len(existingParts) >= 2 {
			if parts[0] == existingParts[0] && parts[1] == existingParts[1] {
				keyExists = true
				break
			}
		}
	}

	if !keyExists {
		// Key no longer exists, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (r *sshAddResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sshAddResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	var state sshAddResourceModel

	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// If the authorized_keys_path or public_key changed, we need to remove the old key and add the new one
	if !plan.AuthorizedKeysPath.Equal(state.AuthorizedKeysPath) || !plan.PublicKey.Equal(state.PublicKey) {
		// Remove the old key first
		err := r.removeKeyFromFile(ctx, state.AuthorizedKeysPath.ValueString(), state.PublicKey.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Failed to remove old key", err.Error())
			return
		}

		// Add the new key
		publicKey := strings.TrimSpace(plan.PublicKey.ValueString())
		keyEntry := publicKey

		if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() && plan.Comment.ValueString() != "" {
			keyEntry = fmt.Sprintf("%s %s", publicKey, plan.Comment.ValueString())
		}

		// Ensure the new authorized_keys directory exists
		authorizedKeysDir := plan.AuthorizedKeysPath.ValueString()
		if strings.Contains(authorizedKeysDir, "/") {
			dirPath := authorizedKeysDir[:strings.LastIndex(authorizedKeysDir, "/")]

			_, err := r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("mkdir -p %s", dirPath))
			if err != nil {
				resp.Diagnostics.AddError("Failed to create authorized_keys directory", err.Error())
				return
			}
		}

		// Append the new key
		_, err = r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("echo '%s' >> %s", keyEntry, plan.AuthorizedKeysPath.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Failed to add new key to authorized_keys", err.Error())
			return
		}

		// Set appropriate permissions
		_, err = r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("chmod 600 %s", plan.AuthorizedKeysPath.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Failed to set permissions on authorized_keys", err.Error())
			return
		}

		// Update ID
		plan.ID = types.StringValue(fmt.Sprintf("%s:%s", plan.AuthorizedKeysPath.ValueString(), publicKey))
	} else if !plan.Comment.Equal(state.Comment) {
		// Only the comment changed, update the key in place
		err := r.updateKeyComment(ctx, plan.AuthorizedKeysPath.ValueString(), plan.PublicKey.ValueString(), plan.Comment.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Failed to update key comment", err.Error())
			return
		}

		// ID stays the same since path and key didn't change
		plan.ID = state.ID
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (r *sshAddResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model sshAddResourceModel

	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Remove the key from the authorized_keys file
	err := r.removeKeyFromFile(ctx, model.AuthorizedKeysPath.ValueString(), model.PublicKey.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to remove key from authorized_keys", err.Error())
		return
	}
}

func (r *sshAddResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Helper function to remove a key from the authorized_keys file
func (r *sshAddResource) removeKeyFromFile(ctx context.Context, filePath, publicKey string) error {
	// Read the current content
	content, err := r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("cat %s 2>/dev/null || echo ''", filePath))
	if err != nil {
		return fmt.Errorf("failed to read authorized_keys file: %w", err)
	}

	publicKey = strings.TrimSpace(publicKey)
	lines := strings.Split(content, "\n")

	var newLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		shouldKeep := true

		if !strings.HasPrefix(line, "#") {
			// Extract just the key part (first two fields typically: key-type and key-data)
			parts := strings.Fields(line)
			existingParts := strings.Fields(publicKey)

			if len(parts) >= 2 && len(existingParts) >= 2 {
				if parts[0] == existingParts[0] && parts[1] == existingParts[1] {
					shouldKeep = false
				}
			}
		}

		if shouldKeep {
			newLines = append(newLines, line)
		}
	}

	// Write the updated content back
	newContent := strings.Join(newLines, "\n")
	if newContent != "" {
		newContent += "\n"
	}

	_, err = r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("echo '%s' > %s", newContent, filePath))
	if err != nil {
		return fmt.Errorf("failed to write updated authorized_keys file: %w", err)
	}

	return nil
}

// Helper function to update the comment for a key
func (r *sshAddResource) updateKeyComment(ctx context.Context, filePath, publicKey, comment string) error {
	// Read the current content
	content, err := r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("cat %s 2>/dev/null || echo ''", filePath))
	if err != nil {
		return fmt.Errorf("failed to read authorized_keys file: %w", err)
	}

	publicKey = strings.TrimSpace(publicKey)
	lines := strings.Split(content, "\n")

	var newLines []string

	for _, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)

		if line == "" {
			newLines = append(newLines, originalLine)
			continue
		}

		if strings.HasPrefix(line, "#") {
			newLines = append(newLines, originalLine)
			continue
		}

		// Check if this is the key we want to update
		parts := strings.Fields(line)
		existingParts := strings.Fields(publicKey)

		if len(parts) >= 2 && len(existingParts) >= 2 {
			if parts[0] == existingParts[0] && parts[1] == existingParts[1] {
				// This is our key, update it with the new comment
				if comment != "" {
					newLines = append(newLines, fmt.Sprintf("%s %s", publicKey, comment))
				} else {
					newLines = append(newLines, publicKey)
				}
			} else {
				newLines = append(newLines, originalLine)
			}
		} else {
			newLines = append(newLines, originalLine)
		}
	}

	// Write the updated content back
	newContent := strings.Join(newLines, "\n")
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	_, err = r.provider.machineAccessClient.RunCommand(ctx, fmt.Sprintf("echo '%s' > %s", newContent, filePath))
	if err != nil {
		return fmt.Errorf("failed to write updated authorized_keys file: %w", err)
	}

	return nil
}
