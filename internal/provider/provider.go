package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
	sshClient *WrapperSshClient
}

// todo: add more validation of the attributes
type providerData struct {
	Private_key types.String `tfsdk:"private_key"`
	User        types.String `tfsdk:"user"`
	Host        types.String `tfsdk:"host"`
	Port        types.String `tfsdk:"port"`
}

// Metadata returns the provider type name.
func (p *internalProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "setup"
}

// Schema defines the provider-level schema for configuration data.
func (p *internalProvider) Schema(ctx context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
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

	p.sshClient, err = createSshClient(data.User.ValueString(), data.Private_key.ValueString(), data.Host.ValueString(), port)
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
	return NewUserResource(p)
}

func (p *internalProvider) newGroupResource() resource.Resource {
	return NewGroupResource(p)
}

func (p *internalProvider) newDirectoryResource() resource.Resource {
	return NewDirectoryResource(p)
}

func (p *internalProvider) newFileResource() resource.Resource {
	return NewFileResource(p)
}

func (p *internalProvider) newAptPackagesResource() resource.Resource {
	return NewAptPackagesResource(p)
}
func (p *internalProvider) newAptRepositoryResource() resource.Resource {
	return NewAptRepositoryResource(p)
}

func (p *internalProvider) createFileWithContent(ctx context.Context, path string, mode string, owner string, group string, content string) error {
	scpClient, err := scp.NewClientBySSH(p.sshClient.Client)
	if err != nil {
		return fmt.Errorf("error creating new SSH session from existing connection.\n %w", err)
	}

	// write file content to tmp file from the host
	tflog.Debug(ctx, "Writing file content to temp file")
	tmpFile, err := os.CreateTemp("", "tempfile")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	tflog.Debug(ctx, "Writing content to temp file "+tmpFile.Name())
	err = os.WriteFile(tmpFile.Name(), []byte(content), 0755)
	if err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	tflog.Debug(ctx, "Copying file to remote host "+path)

	// copy the file to the remote host
	f, _ := os.Open(tmpFile.Name())
	remoteTmpFile, _ := os.CreateTemp("", "tempfile")
	err = scpClient.CopyFromFile(ctx, *f, remoteTmpFile.Name(), "0700")
	if err != nil {
		return fmt.Errorf("failed to copy file to remote host: %w", err)
	}

	// move the file to the correct location
	_, err = p.sshClient.RunCommand(ctx, "sudo mv "+remoteTmpFile.Name()+" "+path)
	if err != nil {
		return err
	}

	// set the owner and group of the remote file
	out, err := p.sshClient.RunCommand(ctx, "sudo chown "+owner+":"+owner+" "+path)
	if err != nil {
		return fmt.Errorf("failed to set owner and group: %s", out)
	}

	// set the mode of the remote file
	out, err = p.sshClient.RunCommand(ctx, "sudo chmod "+mode+" "+path)
	if err != nil {
		return fmt.Errorf("failed to set mode: %s", out)
	}

	return nil
}
