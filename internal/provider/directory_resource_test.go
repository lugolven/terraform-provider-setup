package provider

import (
	"context"
	"fmt"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestDirectoryResource(t *testing.T) {
	const expectedStat = "root root 755\n"

	t.Run("Test create and delete", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDirectoryResourceConfig("/tmp/testdir", "755", 0, 0),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_directory.dir", "path", "/tmp/testdir"),
						resource.TestCheckResourceAttr("setup_directory.dir", "mode", "755"),
						resource.TestCheckResourceAttr("setup_directory.dir", "owner", "0"),
						resource.TestCheckResourceAttr("setup_directory.dir", "group", "0"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							stat, err := sshClient.RunCommand(context.Background(), "stat -c '%U %G %a' /tmp/testdir")
							if err != nil {
								return err
							}

							if stat != expectedStat {
								return fmt.Errorf("unexpected stat: %s", stat)
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

							// check that the directory was deleted
							out, err := sshClient.RunCommand(context.Background(), "ls /tmp/testdir")
							if err == nil {
								return fmt.Errorf("directory was not deleted")
							}

							if out != "ls: cannot access '/tmp/testdir': No such file or directory\n" {
								return fmt.Errorf("unexpected output: %s", out)
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test create, external change and update", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDirectoryResourceConfig("/tmp/testdir", "755", 0, 0),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_directory.dir", "path", "/tmp/testdir"),
						resource.TestCheckResourceAttr("setup_directory.dir", "mode", "755"),
						resource.TestCheckResourceAttr("setup_directory.dir", "owner", "0"),
						resource.TestCheckResourceAttr("setup_directory.dir", "group", "0"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							stat, err := sshClient.RunCommand(context.Background(), "stat -c '%U %G %a' /tmp/testdir")
							if err != nil {
								return err
							}

							if stat != expectedStat {
								return fmt.Errorf("unexpected stat: %s", stat)
							}

							return nil
						},
					),
				},
				{
					PreConfig: func() {
						sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
						if err != nil {
							t.Fatal(err)
						}

						out, err := sshClient.RunCommand(context.Background(), "sudo chmod 777 /tmp/testdir")
						if err != nil {
							t.Fatalf("failed to update directory permissions: %s\n %v", out, err)
						}
					},
					Config: testProviderConfig(setup, "test", "localhost") + testDirectoryResourceConfig("/tmp/testdir", "755", 0, 0),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_directory.dir", "path", "/tmp/testdir"),
						resource.TestCheckResourceAttr("setup_directory.dir", "mode", "755"),
						resource.TestCheckResourceAttr("setup_directory.dir", "owner", "0"),
						resource.TestCheckResourceAttr("setup_directory.dir", "group", "0"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							stat, err := sshClient.RunCommand(context.Background(), "stat -c '%U %G %a' /tmp/testdir")
							if err != nil {
								return err
							}

							if stat != expectedStat {
								return fmt.Errorf("unexpected stat: %s", stat)
							}

							return nil
						},
					),
				},
			},
		})
	})
}

func testDirectoryResourceConfig(path string, mode string, owner int, group int) string {
	return fmt.Sprintf(`
resource "setup_directory" "dir" {
  path  = "%s"
  mode  = "%s"
  owner = %d
  group = %d
}
`, path, mode, owner, group)
}
