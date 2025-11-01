// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &fileDataSource{}
	_ datasource.DataSourceWithConfigure = &fileDataSource{}
)

func newFileDataSource() datasource.DataSource {
	return &fileDataSource{
		provider: nil,
	}
}

type fileDataSource struct {
	provider *internalProvider
}

type fileDataSourceModel struct {
	Path    types.String `tfsdk:"path"`
	Mode    types.String `tfsdk:"mode"`
	Owner   types.Int64  `tfsdk:"owner"`
	Group   types.Int64  `tfsdk:"group"`
	Content types.String `tfsdk:"content"`
	ID      types.String `tfsdk:"id"`
}

func (d *fileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_file"
}

func (d *fileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a file from the remote system via SSH",

		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:    true,
				Description: "The path of the file to read",
			},
			"mode": schema.StringAttribute{
				Computed:    true,
				Description: "The mode of the file",
			},
			"owner": schema.Int64Attribute{
				Computed:    true,
				Description: "The owner UID of the file",
			},
			"group": schema.Int64Attribute{
				Computed:    true,
				Description: "The group GID of the file",
			},
			"content": schema.StringAttribute{
				Computed:    true,
				Description: "The content of the file",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The path of the file (used as ID)",
			},
		},
	}
}

func (d *fileDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	provider, ok := req.ProviderData.(*internalProvider)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			"Expected *internalProvider, got: "+string(rune(0)),
		)

		return
	}

	d.provider = provider
}

func (d *fileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model fileDataSourceModel

	diags := req.Config.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// read the file content
	content, err := d.provider.machineAccessClient.RunCommand(ctx, "sudo cat "+model.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read file", err.Error())
		return
	}

	model.Content = types.StringValue(content)
	model.ID = types.StringValue(model.Path.String())

	// get the file stat
	stat, err := d.provider.machineAccessClient.RunCommand(ctx, "sudo stat -c '%u %g %a' "+model.Path.String())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read file stat", err.Error())
		return
	}

	statParts := strings.Split(strings.Trim(stat, "\n"), " ")

	owner, err := strconv.ParseInt(statParts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse owner", err.Error())
		return
	}

	group, err := strconv.ParseInt(statParts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse group", err.Error())
		return
	}

	model.Owner = types.Int64Value(owner)
	model.Group = types.Int64Value(group)
	model.Mode = types.StringValue(statParts[2])

	diags = resp.State.Set(ctx, &model)
	resp.Diagnostics.Append(diags...)
}
