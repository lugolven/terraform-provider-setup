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

func TestGroupResource(t *testing.T) {
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

		var firstGroupGid string

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
				"setup": providerserver.NewProtocol6WithError(NewProvider()()),
			},
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testGroupResourceConfig("testgroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_group.group", "name", "testgroup"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(t.Context(), "cat /etc/group")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testgroup") {
								return fmt.Errorf("group not found")
							}

							return nil
						},
						func(state *terraform.State) error {
							// get the group id
							groupResource := state.RootModule().Resources["setup_group.group"]
							if groupResource == nil {
								return fmt.Errorf("group resource not found")
							}

							firstGroupGid = groupResource.Primary.Attributes["gid"]
							return nil
						},
					),
				},

				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testGroupResourceConfig("anothergroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_group.group", "name", "anothergroup"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(t.Context(), "cat /etc/group")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "anothergroup") {
								return fmt.Errorf("updated group not found")
							}

							if strings.Contains(content, "testgroup") {
								return fmt.Errorf("old group found")
							}

							return nil
						},

						func(state *terraform.State) error {
							// get the group id
							groupResource := state.RootModule().Resources["setup_group.group"]
							if groupResource == nil {
								return fmt.Errorf("group resource not found")
							}

							if groupResource.Primary.Attributes["gid"] != firstGroupGid {
								return fmt.Errorf("group id changed")
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test already existing group", func(t *testing.T) {
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

		sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		_, err = sshClient.RunCommand(t.Context(), "sudo groupadd testgroup")
		if err != nil {
			t.Fatal(err)
		}

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
				"setup": providerserver.NewProtocol6WithError(NewProvider()()),
			},
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testGroupResourceConfig("testgroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_group.group", "name", "testgroup"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(t.Context(), "cat /etc/group")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testgroup") {
								return fmt.Errorf("group not found")
							}

							return nil
						},
					),
				},
			},
		})
	})
}

func testGroupResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "setup_group" "group" {
	name    = "%s"
}
`, name)
}
