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

func TestUserResource(t *testing.T) {
	t.Run("Test create, update and removed", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		var firstUserGid string

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfig("testuser", "testgroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "cat /etc/passwd")
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
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfig("anotheruser", "testgroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "anotheruser"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "cat /etc/passwd")
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
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfig("testuser", "testgroup"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "cat /etc/passwd")
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

	t.Run("Test removing user from group", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfigWithGroups("testuser", []string{"testgroup", "othergroup"}),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "groups testuser")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testgroup") || !strings.Contains(content, "othergroup") {
								return fmt.Errorf("user not in expected groups")
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfigWithGroups("testuser", []string{"testgroup"}),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "groups testuser")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testgroup") {
								return fmt.Errorf("user not in testgroup")
							}

							if strings.Contains(content, "othergroup") {
								return fmt.Errorf("user still in othergroup")
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

func testUserResourceConfigWithGroups(name string, groupNames []string) string {
	groupResources := ""
	groupRefs := make([]string, len(groupNames))

	for i, groupName := range groupNames {
		groupResources += fmt.Sprintf(`
resource "setup_group" "group%d" {
	name = "%s"
}
`, i, groupName)
		groupRefs[i] = fmt.Sprintf("setup_group.group%d.gid", i)
	}

	return groupResources + fmt.Sprintf(`
resource "setup_user" "user" {
	name    = "%s"
	groups  = [%s]
}
`, name, strings.Join(groupRefs, ", "))
}
