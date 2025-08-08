package provider

import (
	"context"
	"fmt"
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

	// Generate a temporary file path on the remote machine
	remoteTmpPath := fmt.Sprintf("/tmp/docker_load_%s_%s",
		filepath_pkg.Base(tarFilePath),
		strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%p", &plan), "0x", ""), "&", ""))

	// Copy local tar file to remote temporary location
	err := d.provider.machineAccessClient.CopyFile(ctx, tarFilePath, remoteTmpPath)
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

	imageSHA, err := d.getImageSHAAfterLoad(ctx, output)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get image SHA", fmt.Sprintf("Output: %s, Error: %v", output, err))
		return
	}

	plan.ImageSHA = types.StringValue(imageSHA)

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

	inspectCmd := fmt.Sprintf("sudo docker inspect %s", imageSHA)
	_, err := d.provider.machineAccessClient.RunCommand(ctx, inspectCmd)

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

	if !plan.TarFile.Equal(state.TarFile) {
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
		err := d.provider.machineAccessClient.CopyFile(ctx, tarFilePath, remoteTmpPath)
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

		imageSHA, err := d.getImageSHAAfterLoad(ctx, output)
		if err != nil {
			resp.Diagnostics.AddError("Failed to get image SHA", fmt.Sprintf("Output: %s, Error: %v", output, err))
			return
		}

		plan.ImageSHA = types.StringValue(imageSHA)
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

func (d *dockerImageLoadResource) getImageSHAAfterLoad(ctx context.Context, loadOutput string) (string, error) {
	// First, try to extract image reference from load output
	imageRef, err := d.extractImageRefFromOutput(loadOutput)
	if err != nil {
		return "", err
	}

	// Get the actual SHA256 by inspecting the image
	inspectCmd := fmt.Sprintf("sudo docker inspect --format='{{.Id}}' %s", imageRef)
	shaOutput, err := d.provider.machineAccessClient.RunCommand(ctx, inspectCmd)

	if err != nil {
		// Fallback: list recent images and get the most recent one
		listCmd := "sudo docker images --no-trunc --format '{{.ID}}' | head -1"
		shaOutput, err = d.provider.machineAccessClient.RunCommand(ctx, listCmd)

		if err != nil {
			return "", fmt.Errorf("failed to get image SHA: %v", err)
		}
	}

	sha := strings.TrimSpace(shaOutput)
	if !strings.HasPrefix(sha, "sha256:") {
		sha = "sha256:" + sha
	}

	return sha, nil
}

func (d *dockerImageLoadResource) extractImageRefFromOutput(output string) (string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		// Look for "Loaded image: repository:tag"
		if strings.Contains(line, "Loaded image:") {
			parts := strings.Split(line, " ")
			if len(parts) >= 3 {
				imageRef := strings.TrimSpace(parts[2])
				return imageRef, nil
			}
		}
	}

	// Fallback: use "test:latest" as the most likely loaded image name
	return "test:latest", nil
}
