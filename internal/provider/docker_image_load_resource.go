package provider

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	filepath_pkg "path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &dockerImageLoadResource{}
var _ resource.ResourceWithImportState = &dockerImageLoadResource{}

func newDockerImageLoadResource(p *internalProvider) resource.Resource {
	return &dockerImageLoadResource{
		provider: p,
	}
}

type dockerImageLoadResource struct {
	provider *internalProvider
}

type dockerImageLoadResourceModel struct {
	TarFile  types.String `tfsdk:"tar_file"`
	ImageSHA types.String `tfsdk:"image_sha"`
}

func (d *dockerImageLoadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_docker_image_load"
}

func (d *dockerImageLoadResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Loads a Docker image from a tar file and returns the SHA of the loaded image",

		Attributes: map[string]schema.Attribute{
			"tar_file": schema.StringAttribute{
				Required:    true,
				Description: "Path to the tar file containing the Docker image",
			},
			"image_sha": schema.StringAttribute{
				Computed:    true,
				Description: "SHA of the loaded Docker image",
			},
		},
	}
}

func (d *dockerImageLoadResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
}

func (d *dockerImageLoadResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dockerImageLoadResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	tarFilePath := strings.Trim(plan.TarFile.ValueString(), `"`)

	// Check if local tar file exists
	if _, err := os.Stat(tarFilePath); err != nil {
		resp.Diagnostics.AddError("Local tar file not found", fmt.Sprintf("The specified local tar file does not exist: %s", tarFilePath))
		return
	}

	// Get the expected image SHA from the tar file
	expectedImageSHA, err := d.getImageSHAFromLocalTar(tarFilePath)
	if err != nil {
		resp.Diagnostics.AddError("Failed to inspect tar file", fmt.Sprintf("Error reading tar file %s: %v", tarFilePath, err))
		return
	}

	// Generate a temporary file path on the remote machine
	remoteTmpPath := fmt.Sprintf("/tmp/docker_load_%s_%s",
		filepath_pkg.Base(tarFilePath),
		strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%p", &plan), "0x", ""), "&", ""))

	// Copy local tar file to remote temporary location
	err = d.provider.machineAccessClient.CopyFile(ctx, tarFilePath, remoteTmpPath)

	if err != nil {
		resp.Diagnostics.AddError("Failed to copy tar file to remote machine", fmt.Sprintf("Error: %v", err))
		return
	}

	// Ensure cleanup happens even if docker load fails
	defer func() {
		cleanupCmd := fmt.Sprintf("rm -f %s", remoteTmpPath)
		if _, cleanupErr := d.provider.machineAccessClient.RunCommand(ctx, cleanupCmd); cleanupErr != nil {
			resp.Diagnostics.AddWarning("Failed to cleanup temporary file", fmt.Sprintf("Could not remove temporary file %s: %v", remoteTmpPath, cleanupErr))
		}
	}()

	loadCmd := fmt.Sprintf("sudo docker load -i %s 2>&1", remoteTmpPath)
	output, err := d.provider.machineAccessClient.RunCommand(ctx, loadCmd)

	if err != nil {
		resp.Diagnostics.AddError("Failed to load Docker image", fmt.Sprintf("Command: %s, Output: %s, Error: %v", loadCmd, output, err))
		return
	}

	// Verify the image was loaded successfully by checking if it exists
	inspectCmd := fmt.Sprintf("sudo docker inspect %s", expectedImageSHA)
	_, err = d.provider.machineAccessClient.RunCommand(ctx, inspectCmd)

	if err != nil {
		resp.Diagnostics.AddError("Failed to verify loaded Docker image", fmt.Sprintf("Expected image %s was not found after loading. Load output: %s, Error: %v", expectedImageSHA, output, err))
		return
	}

	plan.ImageSHA = types.StringValue(expectedImageSHA)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (d *dockerImageLoadResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dockerImageLoadResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	imageSHA := state.ImageSHA.ValueString()
	tarFilePath := strings.Trim(state.TarFile.ValueString(), `"`)

	// Check if the tar file still exists and get its expected SHA
	if _, err := os.Stat(tarFilePath); err != nil {
		// Tar file no longer exists, remove the resource
		resp.State.RemoveResource(ctx)
		return
	}

	expectedImageSHA, err := d.getImageSHAFromLocalTar(tarFilePath)
	if err != nil {
		// Can't read the tar file, treat as if resource should be removed
		resp.State.RemoveResource(ctx)
		return
	}

	// If the expected SHA differs from current state, the tar content has changed
	// and we need to trigger an update by removing the resource so it gets recreated
	if expectedImageSHA != imageSHA {
		resp.State.RemoveResource(ctx)
		return
	}

	// Check if the image still exists on the remote machine
	inspectCmd := fmt.Sprintf("sudo docker inspect %s", imageSHA)
	_, err = d.provider.machineAccessClient.RunCommand(ctx, inspectCmd)

	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (d *dockerImageLoadResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dockerImageLoadResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	var state dockerImageLoadResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Get the expected image SHA from the tar file
	tarFilePath := strings.Trim(plan.TarFile.ValueString(), `"`)
	expectedImageSHA, err := d.getImageSHAFromLocalTar(tarFilePath)

	if err != nil {
		resp.Diagnostics.AddError("Failed to inspect tar file", fmt.Sprintf("Error reading tar file %s: %v", tarFilePath, err))
		return
	}

	// Check if tar file path changed OR if the expected image SHA differs from current state
	if !plan.TarFile.Equal(state.TarFile) || expectedImageSHA != state.ImageSHA.ValueString() {
		oldImageSHA := state.ImageSHA.ValueString()

		if oldImageSHA != "" {
			removeCmd := fmt.Sprintf("sudo docker rmi %s", oldImageSHA)
			if _, err := d.provider.machineAccessClient.RunCommand(ctx, removeCmd); err != nil {
				resp.Diagnostics.AddWarning("Failed to remove old Docker image", fmt.Sprintf("Could not remove old image %s: %v", oldImageSHA, err))
			}
		}

		tarFilePath := strings.Trim(plan.TarFile.ValueString(), `"`)

		// Check if local tar file exists
		if _, err := os.Stat(tarFilePath); err != nil {
			resp.Diagnostics.AddError("Local tar file not found", fmt.Sprintf("The specified local tar file does not exist: %s", tarFilePath))
			return
		}

		// Generate a temporary file path on the remote machine
		remoteTmpPath := fmt.Sprintf("/tmp/docker_load_%s_%s",
			filepath_pkg.Base(tarFilePath),
			strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%p", &plan), "0x", ""), "&", ""))

		// Copy local tar file to remote temporary location
		err = d.provider.machineAccessClient.CopyFile(ctx, tarFilePath, remoteTmpPath)

		if err != nil {
			resp.Diagnostics.AddError("Failed to copy tar file to remote machine", fmt.Sprintf("Error: %v", err))
			return
		}

		// Ensure cleanup happens even if docker load fails
		defer func() {
			cleanupCmd := fmt.Sprintf("rm -f %s", remoteTmpPath)
			if _, cleanupErr := d.provider.machineAccessClient.RunCommand(ctx, cleanupCmd); cleanupErr != nil {
				resp.Diagnostics.AddWarning("Failed to cleanup temporary file", fmt.Sprintf("Could not remove temporary file %s: %v", remoteTmpPath, cleanupErr))
			}
		}()

		loadCmd := fmt.Sprintf("sudo docker load -i %s 2>&1", remoteTmpPath)
		output, err := d.provider.machineAccessClient.RunCommand(ctx, loadCmd)

		if err != nil {
			resp.Diagnostics.AddError("Failed to load Docker image", err.Error())
			return
		}

		// Verify the image was loaded successfully by checking if it exists
		inspectCmd := fmt.Sprintf("sudo docker inspect %s", expectedImageSHA)
		_, err = d.provider.machineAccessClient.RunCommand(ctx, inspectCmd)

		if err != nil {
			resp.Diagnostics.AddError("Failed to verify loaded Docker image", fmt.Sprintf("Expected image %s was not found after loading. Load output: %s, Error: %v", expectedImageSHA, output, err))
			return
		}

		plan.ImageSHA = types.StringValue(expectedImageSHA)
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}
}

func (d *dockerImageLoadResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dockerImageLoadResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	imageSHA := state.ImageSHA.ValueString()
	if imageSHA != "" {
		removeCmd := fmt.Sprintf("sudo docker rmi %s", imageSHA)
		_, err := d.provider.machineAccessClient.RunCommand(ctx, removeCmd)

		if err != nil {
			resp.Diagnostics.AddError("Failed to remove Docker image", err.Error())
			return
		}
	}
}

func (d *dockerImageLoadResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("image_sha"), req, resp)
}

type dockerManifest struct {
	Config string `json:"Config"`
}

func (d *dockerImageLoadResource) getImageSHAFromLocalTar(tarFilePath string) (string, error) {
	// #nosec G304 - tarFilePath is user-provided and we need to read their specified tar file
	file, err := os.Open(tarFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open tar file: %v", err)
	}
	defer file.Close()

	tarReader := tar.NewReader(file)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return "", fmt.Errorf("failed to read tar file: %v", err)
		}

		if header.Name == "manifest.json" {
			manifestBytes, err := io.ReadAll(tarReader)
			if err != nil {
				return "", fmt.Errorf("failed to read manifest.json: %v", err)
			}

			var manifests []dockerManifest
			if err := json.Unmarshal(manifestBytes, &manifests); err != nil {
				return "", fmt.Errorf("failed to parse manifest.json: %v", err)
			}

			if len(manifests) == 0 {
				return "", fmt.Errorf("no manifests found in manifest.json")
			}

			// Get the config filename, which contains the image SHA
			configFile := manifests[0].Config
			imageSHA := strings.TrimSuffix(configFile, ".json")

			if !strings.HasPrefix(imageSHA, "sha256:") {
				imageSHA = "sha256:" + imageSHA
			}

			return imageSHA, nil
		}
	}

	return "", fmt.Errorf("manifest.json not found in tar file")
}
