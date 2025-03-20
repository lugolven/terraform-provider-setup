package provider

import (
	"fmt"
	"os"
	"strings"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAptPackagesResource(t *testing.T) {
	t.Run("Test create, update and removed", func(t *testing.T) {
		// Arrange
		keyPath, err := os.CreateTemp("", "key")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name())

		if err := clients.CreateSSHKey(t, keyPath.Name()); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name() + ".pub")

		port, stopServer, err := clients.StartDockerSSHServer(t, keyPath.Name()+".pub")
		if err != nil {
			t.Fatal(err)
		}
		defer stopServer()

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
				"setup": providerserver.NewProtocol6WithError(NewProvider()()),
			},
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testAptPackagesResourceConfig([]struct {
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
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							allPackages, err := sshClient.RunCommand(t.Context(), "dpkg -l")
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
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testAptPackagesResourceConfig([]struct {
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
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							allPackages, err := sshClient.RunCommand(t.Context(), "dpkg -l")
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
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testAptPackagesResourceConfig([]struct {
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
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							allPackages, err := sshClient.RunCommand(t.Context(), "dpkg -l")
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
