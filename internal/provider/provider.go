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
	User       types.String `tfsdk:"user"`
	Host       types.String `tfsdk:"host"`
	Port       types.String `tfsdk:"port"`
	PrivateKey types.String `tfsdk:"private_key"`
	SSHAgent   types.String `tfsdk:"ssh_agent"`
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
				Optional:    true,
			},
			"ssh_agent": schema.StringAttribute{
				Description: "Path to the SSH agent socket",
				Optional:    true,
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

	sshClientBuild := clients.CreateSSHMachineAccessClientBuilder(data.User.ValueString(), data.Host.ValueString(), port)
	if data.PrivateKey.ValueString() != "" {
		sshClientBuild.WithPrivateKeyPath(data.PrivateKey.ValueString())
	}

	if data.SSHAgent.ValueString() != "" {
		sshClientBuild.WithAgent(data.SSHAgent.ValueString())
	}

	p.machineAccessClient, err = sshClientBuild.Build(ctx)
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
		p.newDockerImageLoadResource,
		p.newSSHKeyResource,
		p.newSSHAddResource,
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

func (p *internalProvider) newDockerImageLoadResource() resource.Resource {
	return newDockerImageLoadResource(p)
}

func (p *internalProvider) newSSHKeyResource() resource.Resource {
	return newSSHKeyResource(p)
}

func (p *internalProvider) newSSHAddResource() resource.Resource {
	return newSSHAddResource(p)
}
