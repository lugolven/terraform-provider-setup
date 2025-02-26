package provider

import (
	"context"
	"strconv"
	"terraform-provider-setup/internal/provider/clients"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// todo:add integration tests
// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider = &internalProvider{}
)

// NewProvider is a helper function to simplify provider server and testing implementation.
func NewProvider() func() provider.Provider {
	return func() provider.Provider {
		return &internalProvider{}
	}
}

// internalProvider is the provider implementation.
type internalProvider struct {
	machineAccessClient clients.MachineAccessClient
}

// todo: add more validation of the attributes
type providerData struct {
	PrivateKey types.String `tfsdk:"private_key"`
	User       types.String `tfsdk:"user"`
	Host       types.String `tfsdk:"host"`
	Port       types.String `tfsdk:"port"`
}

// Metadata returns the provider type name.
func (p *internalProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "setup"
}

// Schema defines the provider-level schema for configuration data.
func (p *internalProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Sets up bare-metal machines.",
		Attributes: map[string]schema.Attribute{
			"private_key": schema.StringAttribute{
				Description: "Private key to use for SSH authentication",
				Required:    true,
			},
			"user": schema.StringAttribute{
				Description: "User to use for SSH authentication",
				Required:    true,
			},
			"host": schema.StringAttribute{
				Description: "Host to connect to",
				Required:    true,
			},
			"port": schema.StringAttribute{
				Description: "Port to connect to",
				Required:    true,
			},
		},
	}
}

func (p *internalProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerData
	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	port, err := strconv.Atoi(data.Port.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to convert port to int", err.Error())
		return
	}

	p.machineAccessClient, err = clients.CreateSSHMachineAccessClient(data.User.ValueString(), data.PrivateKey.ValueString(), data.Host.ValueString(), port)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create SSH client", err.Error())
		return
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *internalProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

// Resources defines the resources implemented in the provider.
func (p *internalProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		p.newUserResource,
		p.newGroupResource,
		p.newDirectoryResource,
		p.newFileResource,
		p.newAptPackagesResource,
		p.newAptRepositoryResource,
	}
}

// todo: stop passing the provider around, and provide the ssh client as a field
func (p *internalProvider) newUserResource() resource.Resource {
	return newUserResource(p)
}

func (p *internalProvider) newGroupResource() resource.Resource {
	return newGroupResource(p)
}

func (p *internalProvider) newDirectoryResource() resource.Resource {
	return newDirectoryResource(p)
}

func (p *internalProvider) newFileResource() resource.Resource {
	return newFileResource(p)
}

func (p *internalProvider) newAptPackagesResource() resource.Resource {
	return newAptPackagesResource(p)
}
func (p *internalProvider) newAptRepositoryResource() resource.Resource {
	return newAptRepositoryResource(p)
}
