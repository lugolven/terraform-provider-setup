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

func TestSshKeyResource(t *testing.T) {
	// Arrange - set up a single Docker container for all test cases
	setup := setupTestEnvironment(t)

	t.Run("Test create RSA SSH key", func(t *testing.T) {
		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testSSHKeyResourceConfig("/tmp/test_ssh_key_rsa", "rsa", 2048),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_ssh_key.test", "path", "/tmp/test_ssh_key_rsa"),
						resource.TestCheckResourceAttr("setup_ssh_key.test", "key_type", "rsa"),
						resource.TestCheckResourceAttr("setup_ssh_key.test", "key_size", "2048"),
						resource.TestMatchResourceAttr("setup_ssh_key.test", "public_key", regexp.MustCompile("^ssh-rsa AAAA")),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that both private and public key files exist
							_, err = sshClient.RunCommand(context.Background(), "test -f /tmp/test_ssh_key_rsa")
							if err != nil {
								return fmt.Errorf("private key file not found")
							}

							_, err = sshClient.RunCommand(context.Background(), "test -f /tmp/test_ssh_key_rsa.pub")
							if err != nil {
								return fmt.Errorf("public key file not found")
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test create ed25519 SSH key", func(t *testing.T) {
		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testSSHKeyResourceConfigEd25519("/tmp/test_ed25519_key"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_ssh_key.test", "path", "/tmp/test_ed25519_key"),
						resource.TestCheckResourceAttr("setup_ssh_key.test", "key_type", "ed25519"),
						resource.TestMatchResourceAttr("setup_ssh_key.test", "public_key", regexp.MustCompile("^ssh-ed25519 AAAA")),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Verify the public key content starts with ssh-ed25519
							content, err := sshClient.RunCommand(context.Background(), "cat /tmp/test_ed25519_key.pub")
							if err != nil {
								return err
							}

							if !strings.HasPrefix(content, "ssh-ed25519") {
								return fmt.Errorf("expected ed25519 public key, got: %s", content)
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test SSH key with defaults", func(t *testing.T) {
		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testSSHKeyResourceConfigDefaults("/tmp/test_default_key"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_ssh_key.test", "path", "/tmp/test_default_key"),
						resource.TestCheckResourceAttr("setup_ssh_key.test", "key_type", "rsa"),
						resource.TestCheckResourceAttr("setup_ssh_key.test", "key_size", "2048"),
						resource.TestMatchResourceAttr("setup_ssh_key.test", "public_key", regexp.MustCompile("^ssh-rsa AAAA")),
					),
				},
			},
		})
	})
}

func testSSHKeyResourceConfig(path, keyType string, keySize int) string {
	return fmt.Sprintf(`
resource "setup_ssh_key" "test" {
  path     = "%s"
  key_type = "%s"
  key_size = %d
}
`, path, keyType, keySize)
}

func testSSHKeyResourceConfigEd25519(path string) string {
	return fmt.Sprintf(`
resource "setup_ssh_key" "test" {
  path     = "%s"
  key_type = "ed25519"
}
`, path)
}

func testSSHKeyResourceConfigDefaults(path string) string {
	return fmt.Sprintf(`
resource "setup_ssh_key" "test" {
  path = "%s"
}
`, path)
}
