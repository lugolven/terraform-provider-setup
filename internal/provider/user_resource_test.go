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

func TestUserResource(t *testing.T) {
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

		port, stopServer, err := clients.StartDockerSSHServer(t, keyPath.Name()+".pub", keyPath.Name())
		if err != nil {
			t.Fatal(err)
		}
		defer stopServer()

		var firstUserGid string

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
				"setup": providerserver.NewProtocol6WithError(NewProvider()()),
			},
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testUserResourceConfig("testuser", "testgroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(t.Context(), "cat /etc/passwd")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testuser") {
								return fmt.Errorf("user not found")
							}

							return nil
						},
						func(state *terraform.State) error {
							// get the group id
							groupResource := state.RootModule().Resources["setup_user.user"]
							if groupResource == nil {
								return fmt.Errorf("user resource not found")
							}

							firstUserGid = groupResource.Primary.Attributes["uid"]
							return nil
						},
					),
				},
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testUserResourceConfig("anotheruser", "testgroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "anotheruser"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(t.Context(), "cat /etc/passwd")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "anotheruser") {
								return fmt.Errorf("user not found")
							}

							return nil
						},

						func(state *terraform.State) error {
							// get the group id
							groupResource := state.RootModule().Resources["setup_user.user"]
							if groupResource == nil {
								return fmt.Errorf("user resource not found")
							}

							if groupResource.Primary.Attributes["uid"] != firstUserGid {
								return fmt.Errorf("user id changed")
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test with user already created", func(t *testing.T) {
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

		port, stopServer, err := clients.StartDockerSSHServer(t, keyPath.Name()+".pub", keyPath.Name())
		if err != nil {
			t.Fatal(err)
		}
		defer stopServer()

		sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		// create the user
		if out, err := sshClient.RunCommand(t.Context(), "sudo useradd testuser"); err != nil {
			t.Log(out)
			t.Fatal(err)
		}

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
				"setup": providerserver.NewProtocol6WithError(NewProvider()()),
			},
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testUserResourceConfig("testuser", "testgroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(t.Context(), "cat /etc/passwd")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testuser") {
								return fmt.Errorf("user not found")
							}

							return nil
						},
					),
				},
			},
		})
	})
}

func testUserResourceConfig(name string, groupName string) string {
	return fmt.Sprintf(`
resource "setup_group" "group" {
	name    = "%s"
}

resource "setup_user" "user" {
	name    = "%s"
	groups   = [setup_group.group.gid]
}
`, groupName, name)
}
