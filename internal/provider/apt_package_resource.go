// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// todo:add integration tests
// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AptPackageResource{}
var _ resource.ResourceWithImportState = &AptPackageResource{}

func NewAptPackageResource(p *internalProvider) resource.Resource {
	return &AptPackageResource{
		provider: p,
	}
}

// AptPackageResource defines the resource implementation.
type AptPackageResource struct {
	provider *internalProvider
}

type aptPackageResourceModel struct {
	Removed   types.List `tfsdk:"removed"`
	Installed types.List `tfsdk:"installed"`
}

func (aptPackage *AptPackageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apt_package"
}

func (aptPackage *AptPackageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Apt resource",
		Attributes: map[string]schema.Attribute{
			"removed": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The list of apt package to remove",
			},
			"installed": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The list of apt package to install",
			},
		},
	}
}

func (aptPackage *AptPackageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// todo: add validation of the configuration
	// - removed and installed should not be empty at the same time
	// - removed and installed should not have elements in common
}

func (aptPackage *AptPackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aptPackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	currentlyInstalledPackages, err := aptPackage.listCurrentlyInstalledPackages(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list currently installed apt packages", err.Error())
		return
	}
	tflog.Debug(ctx, "Currently installed packages: "+strings.Join(currentlyInstalledPackages, ", "))

	toInsall := []string{}
	for _, element := range plan.Installed.Elements() {
		pkg := element.String()
		pkg = strings.Trim(pkg, "\"")
		if slices.Contains(currentlyInstalledPackages, pkg) {
			tflog.Warn(ctx, "Package "+pkg+" is already installed")
		} else {
			toInsall = append(toInsall, element.String())
		}
	}

	err = aptPackage.ensureInstalled(ctx, toInsall)
	if err != nil {
		resp.Diagnostics.AddError("Failed to install apt packages", err.Error())
		return
	}

	toRemove := []string{}
	for _, element := range plan.Removed.Elements() {
		// element.String() withtout the quotes
		pkg := element.String()
		pkg = strings.Trim(pkg, "\"")
		if slices.Contains(currentlyInstalledPackages, pkg) {
			toRemove = append(toRemove, pkg)
		} else {
			tflog.Warn(ctx, "Package "+pkg+" is not installed")
		}
	}

	err = aptPackage.ensureRemoved(ctx, toRemove)
	if err != nil {
		resp.Diagnostics.AddError("Failed to remove apt packages", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
}

func (aptPackage *AptPackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// todo: implement me
}

func (aptPackage *AptPackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// todo: implement me
}

func (aptPackage *AptPackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// todo: implement me
}

func (aptPackage *AptPackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (aptPackage *AptPackageResource) listCurrentlyInstalledPackages(ctx context.Context) ([]string, error) {
	session, err := aptPackage.provider.sshClient.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	cmd := "sudo apt list --installed"
	tflog.Debug(ctx, "Listing installed apt packages with command: "+cmd)
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed apt packages. Err=%w\nout = %s", err, string(out))
	}

	lines := strings.Split(string(out), "\n")
	packages := []string{}
	for _, line := range lines {
		if strings.Contains(line, "/") {
			packages = append(packages, strings.Split(line, "/")[0])
		}
	}

	return packages, nil
}

func (aptPackage *AptPackageResource) ensureRemoved(ctx context.Context, toRemoved []string) error {
	if len(toRemoved) == 0 {
		tflog.Debug(ctx, "No apt packages to remove")
		return nil
	}
	session, err := aptPackage.provider.sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	cmd := "sudo apt-get remove -y " + strings.Join(toRemoved, " ")

	tflog.Debug(ctx, "Removing apt packages with command: "+cmd)
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("failed to remove apt packages. Err=%w\nout = %s", err, string(out))
	}

	session, err = aptPackage.provider.sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	cmd = "sudo apt autoremove -y"
	tflog.Debug(ctx, "Auto-removing apt packages with command: "+cmd)

	out, err = session.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("failed to auto-remove apt packages. Err=%s\nout = %s", err, string(out))
	}

	return nil
}

func (aptPackage *AptPackageResource) ensureInstalled(ctx context.Context, toInstall []string) error {
	if len(toInstall) == 0 {
		tflog.Debug(ctx, "No apt packages to install")
		return nil
	}

	session, err := aptPackage.provider.sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	cmd := "sudo apt update && sudo apt-get install -y " + strings.Join(toInstall, " ")

	tflog.Debug(ctx, "Installing apt packages with command: "+cmd)
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("failed to install apt packages. Err=%w\nout = %s", err, string(out))
	}

	return nil
}
