package provider

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	filepath_pkg "path/filepath"
	"regexp"
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
	TarFile     types.String `tfsdk:"tar_file"`
	ImageSHA    types.String `tfsdk:"image_sha"`
	ContentHash types.String `tfsdk:"content_hash"`
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
			"content_hash": schema.StringAttribute{
				Computed:    true,
				Description: "Hash of the tar file content for change detection",
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

	// Get the content hash for change detection
	contentHash, err := d.getImageContentHashFromLocalTar(tarFilePath)
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

	// Parse the docker load output to get the actual loaded image reference
	loadedImage := d.parseLoadedImageFromOutput(output)
	if loadedImage == "" {
		resp.Diagnostics.AddError("Failed to parse loaded image from output", fmt.Sprintf("Could not extract loaded image from docker load output: %s", output))
		return
	}

	// Get the actual SHA of the loaded image
	imageSHA, err := d.getImageSHAFromDockerInspect(ctx, loadedImage)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get loaded Docker image SHA", fmt.Sprintf("Could not get SHA for loaded image %s: %v", loadedImage, err))
		return
	}

	plan.ImageSHA = types.StringValue(imageSHA)
	plan.ContentHash = types.StringValue(contentHash)

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

	currentContentHash, err := d.getImageContentHashFromLocalTar(tarFilePath)
	if err != nil {
		// Can't read the tar file, treat as if resource should be removed
		resp.State.RemoveResource(ctx)
		return
	}

	// If the current content hash differs from stored state, the tar content has changed
	// and we need to trigger an update by removing the resource so it gets recreated
	storedContentHash := state.ContentHash.ValueString()
	if currentContentHash != storedContentHash {
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

	// Get the expected image content hash from the tar file
	tarFilePath := strings.Trim(plan.TarFile.ValueString(), `"`)
	expectedContentHash, err := d.getImageContentHashFromLocalTar(tarFilePath)

	if err != nil {
		resp.Diagnostics.AddError("Failed to inspect tar file", fmt.Sprintf("Error reading tar file %s: %v", tarFilePath, err))
		return
	}

	// Get the current tar's content hash
	oldTarFilePath := strings.Trim(state.TarFile.ValueString(), `"`)
	oldContentHash := ""

	if _, err := os.Stat(oldTarFilePath); err == nil {
		oldContentHash, _ = d.getImageContentHashFromLocalTar(oldTarFilePath)
	}

	// Check if tar file path changed OR if the expected content hash differs from current state
	if !plan.TarFile.Equal(state.TarFile) || expectedContentHash != oldContentHash {
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

		// Parse the docker load output to get the actual loaded image reference
		loadedImage := d.parseLoadedImageFromOutput(output)
		if loadedImage == "" {
			resp.Diagnostics.AddError("Failed to parse loaded image from output", fmt.Sprintf("Could not extract loaded image from docker load output: %s", output))
			return
		}

		// Get the actual SHA of the loaded image
		imageSHA, err := d.getImageSHAFromDockerInspect(ctx, loadedImage)
		if err != nil {
			resp.Diagnostics.AddError("Failed to get loaded Docker image SHA", fmt.Sprintf("Could not get SHA for loaded image %s: %v", loadedImage, err))
			return
		}

		plan.ImageSHA = types.StringValue(imageSHA)
		plan.ContentHash = types.StringValue(expectedContentHash)
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

func (d *dockerImageLoadResource) getImageContentHashFromLocalTar(tarFilePath string) (string, error) {
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

			// Use the config file name as a content hash for change detection
			configFile := manifests[0].Config

			return configFile, nil
		}
	}

	return "", fmt.Errorf("manifest.json not found in tar file")
}

func (d *dockerImageLoadResource) parseLoadedImageFromOutput(output string) string {
	// Parse docker load output to extract the loaded image reference
	// Expected format: "Loaded image: <image_reference>" or "Loaded image ID: <sha256>"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Loaded image: ") {
			return strings.TrimPrefix(line, "Loaded image: ")
		}

		if strings.HasPrefix(line, "Loaded image ID: ") {
			return strings.TrimPrefix(line, "Loaded image ID: ")
		}
	}

	// Fallback: try to extract any sha256 hash from the output
	re := regexp.MustCompile(`sha256:[a-f0-9]{64}`)
	if match := re.FindString(output); match != "" {
		return match
	}

	return ""
}

func (d *dockerImageLoadResource) getImageSHAFromDockerInspect(ctx context.Context, imageRef string) (string, error) {
	// Use docker inspect to get the actual SHA of the loaded image
	inspectCmd := fmt.Sprintf("sudo docker inspect --format='{{.Id}}' %s", imageRef)
	output, err := d.provider.machineAccessClient.RunCommand(ctx, inspectCmd)

	if err != nil {
		return "", fmt.Errorf("failed to inspect image %s: %v", imageRef, err)
	}

	imageSHA := strings.TrimSpace(output)
	if imageSHA == "" {
		return "", fmt.Errorf("empty SHA returned from docker inspect for image %s", imageRef)
	}

	// Ensure the SHA has the sha256: prefix
	if !strings.HasPrefix(imageSHA, "sha256:") {
		imageSHA = "sha256:" + imageSHA
	}

	return imageSHA, nil
}
