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
var _ resource.Resource = &AptPackagesResource{}
var _ resource.ResourceWithImportState = &AptPackagesResource{}

func NewAptPackagesResource(p *internalProvider) resource.Resource {
	return &AptPackagesResource{
		provider: p,
	}
}

// AptPackagesResource defines the resource implementation.
type AptPackagesResource struct {
	provider *internalProvider
}

type aptPackagesResourceModel struct {
	Package []*aptPackagesResourcePackageModel `tfsdk:"package"`
}

type aptPackagesResourcePackageModel struct {
	Name   types.String `tfsdk:"name"`
	Absent types.Bool   `tfsdk:"absent"`
}

func (aptPackages *AptPackagesResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apt_packages"
}

func (aptPackages *AptPackagesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Apt packages resource",
		Blocks: map[string]schema.Block{
			"package": schema.ListNestedBlock{
				Description: "Apt package to install or remove",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "The name of the apt package",
						},
						"absent": schema.BoolAttribute{
							Optional:    true,
							Description: "Whether to remove the apt package",
						},
					},
				},
			},
		},
		Attributes: map[string]schema.Attribute{},
	}
}

func (aptPackages *AptPackagesResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// todo: add validation of the configuration
	// - removed and installed should not be empty at the same time
	// - removed and installed should not have elements in common

}

func (aptPackages *AptPackagesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aptPackagesResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	currentlyInstalledPackages, err := aptPackages.listCurrentlyInstalledPackages(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list currently installed apt packages", err.Error())
		return
	}
	tflog.Debug(ctx, "Currently installed packages: "+strings.Join(currentlyInstalledPackages, ", "))

	toInsall := []string{}
	for _, element := range plan.Package {
		if element.Absent.ValueBool() {
			continue
		}

		pkg := element.Name.String()
		pkg = strings.Trim(pkg, "\"")
		if slices.Contains(currentlyInstalledPackages, pkg) {
			tflog.Warn(ctx, "Package "+pkg+" is already installed")
		} else {
			toInsall = append(toInsall, pkg)
		}
	}

	err = aptPackages.ensureInstalled(ctx, toInsall)
	if err != nil {
		resp.Diagnostics.AddError("Failed to install apt packages", err.Error())
		return
	}

	toRemove := []string{}
	for _, element := range plan.Package {
		if element.Absent.ValueBool() {
			continue
		}
		pkg := element.Name.String()
		pkg = strings.Trim(pkg, "\"")
		if slices.Contains(currentlyInstalledPackages, pkg) {
			toRemove = append(toRemove, pkg)
		} else {
			tflog.Warn(ctx, "Package "+pkg+" is not installed")
		}
	}

	err = aptPackages.ensureRemoved(ctx, toRemove)
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

func (aptPackage *AptPackagesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {

}

func (aptPackage *AptPackagesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {

}

func (aptPackage *AptPackagesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan aptPackagesResourceModel
	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	currentlyInstalledPackages, err := aptPackage.listCurrentlyInstalledPackages(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list currently installed apt packages", err.Error())
		return
	}

	toRemove := []string{}
	for _, element := range plan.Package {
		if element.Absent.ValueBool() {
			continue
		}
		pkg := element.Name.String()
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
}

func (aptPackage *AptPackagesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (aptPackage *AptPackagesResource) listCurrentlyInstalledPackages(ctx context.Context) ([]string, error) {

	out, err := aptPackage.provider.machineAccessClient.RunCommand(ctx, "sudo apt list --installed")
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

func (aptPackage *AptPackagesResource) ensureRemoved(ctx context.Context, toRemoved []string) error {
	if len(toRemoved) == 0 {
		tflog.Debug(ctx, "No apt packages to remove")
		return nil
	}

	out, err := aptPackage.provider.machineAccessClient.RunCommand(ctx, "sudo apt-get remove -y "+strings.Join(toRemoved, " "))
	if err != nil {
		return fmt.Errorf("failed to remove apt packages. Err=%w\nout = %s", err, string(out))
	}

	out, err = aptPackage.provider.machineAccessClient.RunCommand(ctx, "sudo apt autoremove -y")
	if err != nil {
		return fmt.Errorf("failed to auto-remove apt packages. Err=%s\nout = %s", err, string(out))
	}

	return nil
}

func (aptPackage *AptPackagesResource) ensureInstalled(ctx context.Context, toInstall []string) error {
	if len(toInstall) == 0 {
		tflog.Debug(ctx, "No apt packages to install")
		return nil
	}

	out, err := aptPackage.provider.machineAccessClient.RunCommand(ctx, "sudo apt update && sudo apt-get install -y "+strings.Join(toInstall, " "))
	if err != nil {
		return fmt.Errorf("failed to install apt packages. Err=%w\nout = %s", err, string(out))
	}

	return nil
}
