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
	// Arrange - set up a single Docker container for all test cases
	setup := setupTestEnvironment(t)

	t.Run("Test create, update and removed", func(t *testing.T) {
		var firstUserGid string

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfig("testuser_create_update", "testgroup_create_update"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser_create_update"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "cat /etc/passwd")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testuser_create_update") {
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
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfig("anotheruser_create_update", "testgroup_create_update"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "anotheruser_create_update"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "cat /etc/passwd")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "anotheruser_create_update") {
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
		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfig("testuser_already_created", "testgroup_already_created"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser_already_created"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "cat /etc/passwd")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testuser_already_created") {
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
		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfigWithGroups("testuser_remove_group", []string{"testgroup_remove_group_1", "testgroup_remove_group_2"}),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser_remove_group"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "groups testuser_remove_group")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testgroup_remove_group_1") || !strings.Contains(content, "testgroup_remove_group_2") {
								return fmt.Errorf("user not in expected groups")
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testUserResourceConfigWithGroups("testuser_remove_group", []string{"testgroup_remove_group_1"}),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_user.user", "name", "testuser_remove_group"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(context.Background(), "groups testuser_remove_group")
							if err != nil {
								return err
							}

							if !strings.Contains(content, "testgroup_remove_group_1") {
								return fmt.Errorf("user not in testgroup_remove_group_1")
							}

							if strings.Contains(content, "testgroup_remove_group_2") {
								return fmt.Errorf("user still in testgroup_remove_group_2")
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
