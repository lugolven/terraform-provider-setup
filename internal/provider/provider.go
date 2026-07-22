package provider

import (
	"context"
	"os"
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

// defaultMaxConcurrentSessions caps how many SSH sessions the provider opens
// against the target host at once. Kept under sshd's default MaxSessions (10)
// so high-parallelism plans/applies don't get sessions rejected with
// "ssh: rejected: connect failed (open failed)".
const defaultMaxConcurrentSessions = 5

// maxConcurrentSessionsEnvVar overrides the default when max_concurrent_sessions
// is not set in the provider configuration.
const maxConcurrentSessionsEnvVar = "SETUP_MAX_CONCURRENT_SESSIONS"

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
	User                  types.String `tfsdk:"user"`
	Host                  types.String `tfsdk:"host"`
	Port                  types.String `tfsdk:"port"`
	PrivateKey            types.String `tfsdk:"private_key"`
	SSHAgent              types.String `tfsdk:"ssh_agent"`
	MaxConcurrentSessions types.Int64  `tfsdk:"max_concurrent_sessions"`
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
			"max_concurrent_sessions": schema.Int64Attribute{
				Description: "Maximum number of SSH sessions opened concurrently against the " +
					"target host. Keep at or below the host sshd's MaxSessions (default 10) to " +
					"avoid 'ssh: rejected: connect failed (open failed)' during high-parallelism " +
					"runs. Set to 0 to disable the limit. Can also be set via the " +
					maxConcurrentSessionsEnvVar + " environment variable. Defaults to 5.",
				Optional: true,
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

	maxConcurrentSessions, ok := resolveMaxConcurrentSessions(data.MaxConcurrentSessions, resp)
	if !ok {
		return
	}

	sshClientBuild := clients.CreateSSHMachineAccessClientBuilder(data.User.ValueString(), data.Host.ValueString(), port)
	sshClientBuild.WithMaxConcurrentSessions(maxConcurrentSessions)

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

// resolveMaxConcurrentSessions determines the concurrent-session limit from the
// provider configuration, falling back to the SETUP_MAX_CONCURRENT_SESSIONS
// environment variable and finally the built-in default. It rejects negative
// values (0 is a valid "unlimited" escape hatch). The bool result is false when
// a diagnostic error was reported and configuration should abort.
func resolveMaxConcurrentSessions(cfg types.Int64, resp *provider.ConfigureResponse) (int, bool) {
	maxConcurrentSessions := defaultMaxConcurrentSessions

	switch {
	case !cfg.IsNull() && !cfg.IsUnknown():
		maxConcurrentSessions = int(cfg.ValueInt64())
	default:
		if env := os.Getenv(maxConcurrentSessionsEnvVar); env != "" {
			parsed, err := strconv.Atoi(env)
			if err != nil {
				resp.Diagnostics.AddError(
					"Invalid "+maxConcurrentSessionsEnvVar,
					"Failed to parse "+maxConcurrentSessionsEnvVar+" as an integer: "+err.Error(),
				)

				return 0, false
			}

			maxConcurrentSessions = parsed
		}
	}

	if maxConcurrentSessions < 0 {
		resp.Diagnostics.AddError(
			"Invalid max_concurrent_sessions",
			"max_concurrent_sessions must be >= 0 (0 disables the limit).",
		)

		return 0, false
	}

	return maxConcurrentSessions, true
}

// DataSources defines the data sources implemented in the provider.
func (p *internalProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		p.newFileDataSource,
	}
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

func (p *internalProvider) newFileDataSource() datasource.DataSource {
	return newFileDataSource(p)
}
