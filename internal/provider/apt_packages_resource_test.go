package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAptPackagesResource(t *testing.T) {
	t.Run("Test create, update and removed", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptPackagesResourceConfig([]struct {
						name   string
						absent bool
					}{
						{
							name:   "curl",
							absent: false,
						},
						{
							name:   "vlc",
							absent: true,
						},
					}),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.0.name", "curl"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.0.absent", "false"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.1.name", "vlc"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.1.absent", "true"),

						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							allPackages, err := sshClient.RunCommand(context.Background(), "dpkg -l")
							if err != nil {
								return fmt.Errorf("error when running ''dpkg -l'': %w", err)
							}

							if !strings.Contains(allPackages, "curl") {
								return fmt.Errorf("package curl not found")
							}

							if strings.Contains(allPackages, "vlc") {
								return fmt.Errorf("package vlc found")
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptPackagesResourceConfig([]struct {
						name   string
						absent bool
					}{
						{
							name:   "vlc",
							absent: true,
						},
					}),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.0.name", "vlc"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.0.absent", "true"),

						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							allPackages, err := sshClient.RunCommand(context.Background(), "dpkg -l")
							if err != nil {
								return fmt.Errorf("error when running ''dpkg -l'': %w", err)
							}

							if strings.Contains(allPackages, "curl") {
								return fmt.Errorf("package curl found")
							}

							if strings.Contains(allPackages, "vlc") {
								return fmt.Errorf("package vlc found")
							}

							return nil
						},
					),
				},

				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptPackagesResourceConfig([]struct {
						name   string
						absent bool
					}{
						{
							name:   "vlc",
							absent: true,
						},
						{
							name:   "git",
							absent: false,
						},
						{
							name:   "nmap",
							absent: false,
						},
					}),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.0.name", "vlc"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.0.absent", "true"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.1.name", "git"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.1.absent", "false"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.2.name", "nmap"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.2.absent", "false"),

						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							allPackages, err := sshClient.RunCommand(context.Background(), "dpkg -l")
							if err != nil {
								return fmt.Errorf("error when running ''dpkg -l'': %w", err)
							}

							if strings.Contains(allPackages, "vlc") {
								return fmt.Errorf("package vlc found")
							}

							if !strings.Contains(allPackages, "git") {
								return fmt.Errorf("package git not found")
							}

							if !strings.Contains(allPackages, "nmap") {
								return fmt.Errorf("package nmap not found")
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test package does not exist", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptPackagesResourceConfig([]struct {
						name   string
						absent bool
					}{
						{
							name:   "nonexistent-package-xyz123",
							absent: false,
						},
					}),
					ExpectError: func() *regexp.Regexp {
						return regexp.MustCompile(".*Unable to locate package.*|.*Package.*not found.*|.*No such package.*")
					}(),
				},
			},
		})
	})

	t.Run("Test remove non-existent package", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptPackagesResourceConfig([]struct {
						name   string
						absent bool
					}{
						{
							name:   "another-nonexistent-package-abc456",
							absent: true,
						},
					}),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.0.name", "another-nonexistent-package-abc456"),
						resource.TestCheckResourceAttr("setup_apt_packages.packages", "package.0.absent", "true"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							allPackages, err := sshClient.RunCommand(context.Background(), "dpkg -l")
							if err != nil {
								return fmt.Errorf("error when running ''dpkg -l'': %w", err)
							}

							if strings.Contains(allPackages, "another-nonexistent-package-abc456") {
								return fmt.Errorf("package another-nonexistent-package-abc456 should not be found")
							}

							return nil
						},
					),
				},
			},
		})
	})
}

func testAptPackagesResourceConfig(packages []struct {
	name   string
	absent bool
}) string {
	var packagesConfig string
	for _, p := range packages {
		packagesConfig += fmt.Sprintf(`
		  package {
		  	name = "%s"
			absent = %t
		}
`, p.name, p.absent)
	}

	return fmt.Sprintf(`
resource "setup_apt_packages" "packages" {
		%s
}
`, packagesConfig)
}
