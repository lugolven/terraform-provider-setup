package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAptRepositoryResource(t *testing.T) {
	t.Run("Test create, update and delete", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithHTTPKey(t, "docker", "https://download.docker.com/linux/ubuntu", "https://download.docker.com/linux/ubuntu/gpg"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "docker"),
						resource.TestCheckResourceAttrSet("setup_apt_repository.repo", "key"),
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "url", "https://download.docker.com/linux/ubuntu"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check if key file exists
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/docker.asc")
							if err != nil {
								return fmt.Errorf("key file not found: %w", err)
							}

							// Check if source list exists
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/docker.list")
							if err != nil {
								return fmt.Errorf("source list not found: %w", err)
							}

							// Verify content of source list
							content, err := sshClient.RunCommand(context.Background(), "cat /etc/apt/sources.list.d/docker.list")
							if err != nil {
								return fmt.Errorf("failed to read source list: %w", err)
							}

							if content == "" {
								return fmt.Errorf("source list is empty")
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithHTTPKey(t, "docker", "https://download.docker.com/linux/debian", "https://download.docker.com/linux/debian/gpg"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "docker"),
						resource.TestCheckResourceAttrSet("setup_apt_repository.repo", "key"),
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "url", "https://download.docker.com/linux/debian"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Verify updated content of source list
							content, err := sshClient.RunCommand(context.Background(), "cat /etc/apt/sources.list.d/docker.list")
							if err != nil {
								return fmt.Errorf("failed to read source list: %w", err)
							}

							if content == "" {
								return fmt.Errorf("source list is empty")
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost"),
					Check: resource.ComposeTestCheckFunc(
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that key file was deleted
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/docker.asc")
							if err == nil {
								return fmt.Errorf("key file should have been deleted")
							}

							// Check that source list was deleted
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/docker.list")
							if err == nil {
								return fmt.Errorf("source list should have been deleted")
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test name change", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithHTTPKey(t, "old-repo", "https://download.docker.com/linux/ubuntu", "https://download.docker.com/linux/ubuntu/gpg"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "old-repo"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check if old files exist
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/old-repo.asc")
							if err != nil {
								return fmt.Errorf("old key file not found: %w", err)
							}

							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/old-repo.list")
							if err != nil {
								return fmt.Errorf("old source list not found: %w", err)
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithHTTPKey(t, "new-repo", "https://download.docker.com/linux/ubuntu", "https://download.docker.com/linux/ubuntu/gpg"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "new-repo"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that old files were removed
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/old-repo.asc")
							if err == nil {
								return fmt.Errorf("old key file should have been removed")
							}

							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/old-repo.list")
							if err == nil {
								return fmt.Errorf("old source list should have been removed")
							}

							// Check that new files exist
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/new-repo.asc")
							if err != nil {
								return fmt.Errorf("new key file not found: %w", err)
							}

							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/new-repo.list")
							if err != nil {
								return fmt.Errorf("new source list not found: %w", err)
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test read with missing files", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithHTTPKey(t, "test-repo", "https://download.docker.com/linux/ubuntu", "https://download.docker.com/linux/ubuntu/gpg"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "test-repo"),
					),
				},
				{
					PreConfig: func() {
						sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
						if err != nil {
							t.Fatal(err)
						}

						// Manually remove key file to simulate external change
						_, err = sshClient.RunCommand(context.Background(), "sudo rm -f /etc/apt/keyrings/test-repo.asc")
						if err != nil {
							t.Fatalf("failed to remove key file: %v", err)
						}
					},
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithHTTPKey(t, "test-repo", "https://download.docker.com/linux/ubuntu", "https://download.docker.com/linux/ubuntu/gpg"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "test-repo"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that key file was recreated
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/test-repo.asc")
							if err != nil {
								return fmt.Errorf("key file should have been recreated: %w", err)
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test repository validation - invalid URL should fail", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithHTTPKey(t, "invalid-repo", "https://invalid-repository-url.example.com/linux/debian", "https://download.docker.com/linux/debian/gpg"),
					ExpectError: regexp.MustCompile("Failed to update apt package cache after adding repository|Repository validation failed"),
				},
			},
		})
	})

	t.Run("Test repository validation - Debian URL on Ubuntu should fail", func(t *testing.T) {
		// Arrange  
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithHTTPKey(t, "debian-on-ubuntu", "https://download.docker.com/linux/debian", "https://download.docker.com/linux/debian/gpg"),
					ExpectError: regexp.MustCompile("Failed to update apt package cache after adding repository|Repository validation failed|Failed to fetch"),
				},
			},
		})
	})

	t.Run("Test repository validation - malformed GPG key should fail", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)
		invalidGpgKey := "-----BEGIN PGP PUBLIC KEY BLOCK-----\ninvalid key content\n-----END PGP PUBLIC KEY BLOCK-----"

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfigWithStaticKey("invalid-key", invalidGpgKey, "https://download.docker.com/linux/debian"),
					ExpectError: regexp.MustCompile("Failed to update apt package cache after adding repository|Repository validation failed|NO_PUBKEY"),
				},
			},
		})
	})
}

func testAptRepositoryResourceConfigWithHTTPKey(t *testing.T, name string, url string, keyURL string) string {
	t.Helper()
	
	// Fetch GPG key via HTTP
	// #nosec G107 - This is a test function using trusted test URLs
	resp, err := http.Get(keyURL)
	if err != nil {
		t.Fatalf("Failed to fetch GPG key from %s: %v", keyURL, err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP request failed with status %d for URL %s", resp.StatusCode, keyURL)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	
	key := string(body)
	
	return fmt.Sprintf(`
resource "setup_apt_repository" "repo" {
	name = "%s"
	key  = <<EOT
%s
EOT
	url  = "%s"
}
`, name, key, url)
}

func testAptRepositoryResourceConfigWithStaticKey(name string, key string, url string) string {
	return fmt.Sprintf(`
resource "setup_apt_repository" "repo" {
	name = "%s"
	key  = <<EOT
%s
EOT
	url  = "%s"
}
`, name, key, url)
}
