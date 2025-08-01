// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-setup/internal/provider/clients"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &lineinfileResource{}
var _ resource.ResourceWithImportState = &lineinfileResource{}

func newLineinfileResource(p *internalProvider) resource.Resource {
	return &lineinfileResource{
		provider: p,
	}
}

// lineinfileResource defines the resource implementation.
type lineinfileResource struct {
	provider *internalProvider
}

type lineinfileResourceModel struct {
	Path   types.String `tfsdk:"path"`
	Line   types.String `tfsdk:"line"`
	Regexp types.String `tfsdk:"regexp"`
	ID     types.String `tfsdk:"id"`
}

func (r *lineinfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lineinfile"
}

func (r *lineinfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Ensure lines are present in existing text files",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"path": schema.StringAttribute{
				Required:    true,
				Description: "The path of the file to modify",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"line": schema.StringAttribute{
				Required:    true,
				Description: "The line to insert/replace in the file",
			},
			"regexp": schema.StringAttribute{
				Optional:    true,
				Description: "The regular expression to look for in every line of the file",
			},
		},
	}
}

func (r *lineinfileResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
}

func (r *lineinfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lineinfileResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// No default values needed

	// Generate ID
	plan.ID = types.StringValue(plan.Path.ValueString())

	err := r.ensureLine(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Failed to manage line in file", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *lineinfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lineinfileResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Check if file exists and read content for line existence check
	fileInfo, err := r.provider.machineAccessClient.ReadFile(ctx, state.Path.ValueString())
	if err != nil {
		var fileNotFoundErr clients.FileNotFoundError
		if errors.As(err, &fileNotFoundErr) {
			// File doesn't exist, remove from state
			resp.State.RemoveResource(ctx)
			return
		}
		// Other read errors should be reported
		resp.Diagnostics.AddError("Failed to read file", err.Error())
		return
	}

	lines := strings.Split(fileInfo.Content, "\n")
	lineExists := false
	line := state.Line.ValueString()

	if !state.Regexp.IsNull() && state.Regexp.ValueString() != "" {
		// Check using regexp
		regexpStr := state.Regexp.ValueString()
		regex, err := regexp.Compile(regexpStr)
		if err != nil {
			resp.Diagnostics.AddError("Invalid regexp", err.Error())
			return
		}
		for _, fileLine := range lines {
			if regex.MatchString(fileLine) {
				lineExists = true
				break
			}
		}
	} else {
		// Check for exact line match
		for _, fileLine := range lines {
			if fileLine == line {
				lineExists = true
				break
			}
		}
	}

	if !lineExists {
		// State has drifted, but keep the resource in state to allow correction on next apply
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *lineinfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan lineinfileResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	err := r.ensureLine(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update line in file", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *lineinfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lineinfileResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// On delete, do nothing - preserve the file content
	// The line remains in the file even after the resource is destroyed
}

func (r *lineinfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("path"), req, resp)
}

// ensureLine manages the line in the file according to the configuration
func (r *lineinfileResource) ensureLine(ctx context.Context, model *lineinfileResourceModel) error {
	filePath := model.Path.ValueString()
	line := model.Line.ValueString()

	// Read the file content and metadata
	fileInfo, err := r.provider.machineAccessClient.ReadFile(ctx, filePath)
	if err != nil {
		var fileNotFoundErr clients.FileNotFoundError
		if errors.As(err, &fileNotFoundErr) {
			return fmt.Errorf("file %s does not exist", filePath)
		}
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	lines := strings.Split(fileInfo.Content, "\n")

	// Check if line already exists
	if !model.Regexp.IsNull() && model.Regexp.ValueString() != "" {
		// Check using regexp
		regexpStr := model.Regexp.ValueString()
		regex, err := regexp.Compile(regexpStr)
		if err != nil {
			return fmt.Errorf("invalid regexp %s: %w", regexpStr, err)
		}

		// Look for existing line matching regexp
		for i, fileLine := range lines {
			if regex.MatchString(fileLine) {
				if fileLine == line {
					// Line already exists and matches, nothing to do
					return nil
				}
				// Replace the existing line
				lines[i] = line
				newContent := strings.Join(lines, "\n")
				return r.provider.machineAccessClient.WriteFile(ctx, filePath, fileInfo.Mode, fileInfo.Owner, fileInfo.Group, newContent)
			}
		}
		// No line matching regexp found, append to end
	} else {
		// Check for exact line match
		for _, fileLine := range lines {
			if fileLine == line {
				// Line already exists, nothing to do
				return nil
			}
		}
	}

	// Line doesn't exist, append to end of file
	newLines := append(lines, line)
	newContent := strings.Join(newLines, "\n")
	return r.provider.machineAccessClient.WriteFile(ctx, filePath, fileInfo.Mode, fileInfo.Owner, fileInfo.Group, newContent)
}
