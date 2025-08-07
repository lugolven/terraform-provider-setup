package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

	t.Run("Test install docker via repository", func(t *testing.T) {
		// Download Docker GPG key dynamically
		resp, err := http.Get("https://download.docker.com/linux/ubuntu/gpg")
		if err != nil {
			t.Fatalf("Failed to download Docker GPG key: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to download Docker GPG key: HTTP %d", resp.StatusCode)
		}

		keyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read Docker GPG key: %v", err)
		}

		dockerGpgKey := string(keyBytes)

		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDockerRepositoryAndPackageConfig(dockerGpgKey),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.docker", "name", "docker"),
						resource.TestCheckResourceAttr("setup_apt_repository.docker", "url", "https://download.docker.com/linux/ubuntu"),
						resource.TestCheckResourceAttr("setup_apt_packages.docker_packages", "package.0.name", "docker-ce"),
						resource.TestCheckResourceAttr("setup_apt_packages.docker_packages", "package.0.absent", "false"),

						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check if Docker repository was set up correctly
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/docker.asc")
							if err != nil {
								return fmt.Errorf("docker key file not found: %w", err)
							}

							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/docker.list")
							if err != nil {
								return fmt.Errorf("docker source list not found: %w", err)
							}

							// Verify the repository configuration was created properly
							sourceContent, err := sshClient.RunCommand(context.Background(), "cat /etc/apt/sources.list.d/docker.list")
							if err != nil {
								return fmt.Errorf("failed to read docker source list: %w", err)
							}

							if !strings.Contains(sourceContent, "download.docker.com") {
								return fmt.Errorf("docker repository URL not found in source list")
							}

							// Check if Docker package was installed
							allPackages, err := sshClient.RunCommand(context.Background(), "dpkg -l")
							if err != nil {
								return fmt.Errorf("error when running 'dpkg -l': %w", err)
							}

							if !strings.Contains(allPackages, "docker-ce") {
								return fmt.Errorf("docker-ce package not found")
							}

							// Verify Docker service is available (not necessarily running)
							_, err = sshClient.RunCommand(context.Background(), "which docker")
							if err != nil {
								return fmt.Errorf("docker binary not found in PATH: %w", err)
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

func testDockerRepositoryAndPackageConfig(dockerGpgKey string) string {
	return fmt.Sprintf(`
resource "setup_apt_repository" "docker" {
	name = "docker"
	key  = <<EOT
%s
EOT
	url  = "https://download.docker.com/linux/ubuntu"
}

resource "setup_apt_packages" "docker_packages" {
	depends_on = [setup_apt_repository.docker]
	
	package {
		name = "docker-ce"
		absent = false
	}
}
`, dockerGpgKey)
}
