package provider

import (
	"context"
	"fmt"
	"strings"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestSSHAddResource(t *testing.T) {
	t.Run("Test add SSH public key to authorized_keys", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testSSHAddResourceConfig("/tmp/authorized_keys", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7vbqaj", "test-comment"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_ssh_add.test", "authorized_keys_path", "/tmp/authorized_keys"),
						resource.TestCheckResourceAttr("setup_ssh_add.test", "public_key", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7vbqaj"),
						resource.TestCheckResourceAttr("setup_ssh_add.test", "comment", "test-comment"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that the key was added to authorized_keys
							content, err := sshClient.RunCommand(context.Background(), "cat /tmp/authorized_keys")
							if err != nil {
								return fmt.Errorf("authorized_keys file not found")
							}

							expectedEntry := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7vbqaj test-comment"
							if !strings.Contains(content, expectedEntry) {
								return fmt.Errorf("expected key entry not found in authorized_keys: %s", content)
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test add SSH public key without comment", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testSSHAddResourceConfigNoComment("/tmp/authorized_keys_no_comment", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGbA8VjAq"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_ssh_add.test", "authorized_keys_path", "/tmp/authorized_keys_no_comment"),
						resource.TestCheckResourceAttr("setup_ssh_add.test", "public_key", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGbA8VjAq"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that the key was added to authorized_keys
							content, err := sshClient.RunCommand(context.Background(), "cat /tmp/authorized_keys_no_comment")
							if err != nil {
								return fmt.Errorf("authorized_keys file not found")
							}

							expectedKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGbA8VjAq"
							if !strings.Contains(content, expectedKey) {
								return fmt.Errorf("expected key not found in authorized_keys: %s", content)
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test update SSH key comment", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testSSHAddResourceConfig("/tmp/authorized_keys_update", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7vbqaj", "initial-comment"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_ssh_add.test", "comment", "initial-comment"),
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testSSHAddResourceConfig("/tmp/authorized_keys_update", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7vbqaj", "updated-comment"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_ssh_add.test", "comment", "updated-comment"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that the comment was updated
							content, err := sshClient.RunCommand(context.Background(), "cat /tmp/authorized_keys_update")
							if err != nil {
								return fmt.Errorf("authorized_keys file not found")
							}

							expectedEntry := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7vbqaj updated-comment"
							if !strings.Contains(content, expectedEntry) {
								return fmt.Errorf("expected updated key entry not found in authorized_keys: %s", content)
							}

							// Ensure old comment is not present
							oldEntry := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7vbqaj initial-comment"
							if strings.Contains(content, oldEntry) {
								return fmt.Errorf("old comment still present in authorized_keys: %s", content)
							}

							return nil
						},
					),
				},
			},
		})
	})
}

func testSSHAddResourceConfig(authorizedKeysPath, publicKey, comment string) string {
	return fmt.Sprintf(`
resource "setup_ssh_add" "test" {
  authorized_keys_path = "%s"
  public_key           = "%s"
  comment              = "%s"
}
`, authorizedKeysPath, publicKey, comment)
}

func testSSHAddResourceConfigNoComment(authorizedKeysPath, publicKey string) string {
	return fmt.Sprintf(`
resource "setup_ssh_add" "test" {
  authorized_keys_path = "%s"
  public_key           = "%s"
}
`, authorizedKeysPath, publicKey)
}
