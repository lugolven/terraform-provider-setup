package provider

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
	// Docker client will be created on-demand for each operation
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
	contentHash, err := d.getImageContentHashFromLocalTar(ctx, tarFilePath)
	if err != nil {
		resp.Diagnostics.AddError("Failed to inspect tar file", fmt.Sprintf("Error reading tar file %s: %v", tarFilePath, err))
		return
	}

	// Load the Docker image using remote Docker socket via SSH
	imageSHA, err := d.loadImageUsingRemoteDocker(ctx, tarFilePath)
	if err != nil {
		resp.Diagnostics.AddError("Failed to load Docker image", fmt.Sprintf("Error: %v", err))
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

	currentContentHash, err := d.getImageContentHashFromLocalTar(ctx, tarFilePath)
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
	if !d.imageExistsRemotely(ctx, imageSHA) {
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
	expectedContentHash, err := d.getImageContentHashFromLocalTar(ctx, tarFilePath)

	if err != nil {
		resp.Diagnostics.AddError("Failed to inspect tar file", fmt.Sprintf("Error reading tar file %s: %v", tarFilePath, err))
		return
	}

	// Get the current tar's content hash
	oldTarFilePath := strings.Trim(state.TarFile.ValueString(), `"`)
	oldContentHash := ""

	if _, err := os.Stat(oldTarFilePath); err == nil {
		oldContentHash, _ = d.getImageContentHashFromLocalTar(ctx, oldTarFilePath)
	}

	// Check if tar file path changed OR if the expected content hash differs from current state
	if !plan.TarFile.Equal(state.TarFile) || expectedContentHash != oldContentHash {
		oldImageSHA := state.ImageSHA.ValueString()

		if oldImageSHA != "" {
			if err := d.removeImageRemotely(ctx, oldImageSHA); err != nil {
				resp.Diagnostics.AddWarning("Failed to remove old Docker image", fmt.Sprintf("Could not remove old image %s: %v", oldImageSHA, err))
			}
		}

		tarFilePath := strings.Trim(plan.TarFile.ValueString(), `"`)

		// Check if local tar file exists
		if _, err := os.Stat(tarFilePath); err != nil {
			resp.Diagnostics.AddError("Local tar file not found", fmt.Sprintf("The specified local tar file does not exist: %s", tarFilePath))
			return
		}

		// Load the Docker image using remote Docker socket via SSH
		imageSHA, err := d.loadImageUsingRemoteDocker(ctx, tarFilePath)
		if err != nil {
			resp.Diagnostics.AddError("Failed to load Docker image", fmt.Sprintf("Error: %v", err))
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
		if err := d.removeImageRemotely(ctx, imageSHA); err != nil {
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

func (d *dockerImageLoadResource) getImageContentHashFromLocalTar(ctx context.Context, tarFilePath string) (string, error) {
	tflog.Debug(ctx, "Getting the sha of the local tar file")

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

func (d *dockerImageLoadResource) loadImageUsingRemoteDocker(ctx context.Context, tarFilePath string) (string, error) {
	// Create Docker client on-demand using the machine access client
	dockerClient, err := d.provider.machineAccessClient.GetDockerClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create Docker client: %v", err)
	}

	// Open the tar file for loading
	// #nosec G304 - tarFilePath is user-provided and we need to read their specified tar file
	tarFile, err := os.Open(tarFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open tar file: %v", err)
	}
	defer tarFile.Close()

	// Load the image using Docker SDK
	response, err := dockerClient.ImageLoad(ctx, tarFile, true)
	if err != nil {
		return "", fmt.Errorf("failed to load image via Docker API: %v", err)
	}
	defer response.Body.Close()

	// Read the response to get load details
	responseBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read load response: %v", err)
	}

	// Parse the output to get the loaded image reference
	loadedImage := d.parseLoadedImageFromOutput(string(responseBytes))
	if loadedImage == "" {
		return "", fmt.Errorf("could not extract loaded image from docker load output: %s", string(responseBytes))
	}

	// Get the actual SHA of the loaded image using Docker API
	imageInspect, _, err := dockerClient.ImageInspectWithRaw(ctx, loadedImage)
	if err != nil {
		return "", fmt.Errorf("failed to inspect loaded image %s: %v", loadedImage, err)
	}

	return imageInspect.ID, nil
}

func (d *dockerImageLoadResource) imageExistsRemotely(ctx context.Context, imageSHA string) bool {
	// Create Docker client on-demand using the machine access client
	dockerClient, err := d.provider.machineAccessClient.GetDockerClient(ctx)
	if err != nil {
		return false
	}

	_, _, err = dockerClient.ImageInspectWithRaw(ctx, imageSHA)

	return err == nil
}

func (d *dockerImageLoadResource) removeImageRemotely(ctx context.Context, imageSHA string) error {
	// Create Docker client on-demand using the machine access client
	dockerClient, err := d.provider.machineAccessClient.GetDockerClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}

	_, err = dockerClient.ImageRemove(ctx, imageSHA, dockertypes.ImageRemoveOptions{})

	return err
}

func (d *dockerImageLoadResource) parseLoadedImageFromOutput(output string) string {
	// Parse docker load output to extract the loaded image reference
	// For Docker SDK, the output may be in JSON streaming format: {"stream":"Loaded image: <image_reference>\n"}
	// For raw command output: "Loaded image: <image_reference>" or "Loaded image ID: <sha256>"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			var streamOutput struct {
				Stream string `json:"stream"`
			}

			if err := json.Unmarshal([]byte(line), &streamOutput); err == nil {
				if strings.HasPrefix(streamOutput.Stream, "Loaded image: ") {
					return strings.TrimPrefix(strings.TrimSpace(streamOutput.Stream), "Loaded image: ")
				}

				if strings.HasPrefix(streamOutput.Stream, "Loaded image ID: ") {
					return strings.TrimPrefix(strings.TrimSpace(streamOutput.Stream), "Loaded image ID: ")
				}
			}
		}

		// Fallback to raw format parsing
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
