package provider

import (
	"fmt"
	"os"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestFileResource(t *testing.T) {
	const expectedStat = "root root 644\n"

	t.Run("Test create", func(t *testing.T) {
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

		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
				"setup": providerserver.NewProtocol6WithError(NewProvider()()),
			},
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testFileResourceConfig("/tmp/test.txt", "0644", 0, 0, "hello world"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_file.file", "path", "/tmp/test.txt"),
						resource.TestCheckResourceAttr("setup_file.file", "mode", "0644"),
						resource.TestCheckResourceAttr("setup_file.file", "owner", "0"),
						resource.TestCheckResourceAttr("setup_file.file", "group", "0"),
						resource.TestCheckResourceAttr("setup_file.file", "content", "hello world"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(t.Context(), "cat /tmp/test.txt")
							if err != nil {
								return err
							}

							if content != "hello world" {
								return fmt.Errorf("unexpected content: %s", content)
							}

							// check owner and group and mode
							stat, err := sshClient.RunCommand(t.Context(), "stat -c '%U %G %a' /tmp/test.txt")
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
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) + testFileResourceConfig("/tmp/test.txt", "0644", 0, 0, "world hello"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_file.file", "path", "/tmp/test.txt"),
						resource.TestCheckResourceAttr("setup_file.file", "mode", "0644"),
						resource.TestCheckResourceAttr("setup_file.file", "owner", "0"),
						resource.TestCheckResourceAttr("setup_file.file", "group", "0"),
						resource.TestCheckResourceAttr("setup_file.file", "content", "world hello"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							content, err := sshClient.RunCommand(t.Context(), "cat /tmp/test.txt")
							if err != nil {
								return err
							}

							if content != "world hello" {
								return fmt.Errorf("unexpected content: %s", content)
							}

							// check owner and group and mode
							stat, err := sshClient.RunCommand(t.Context(), "stat -c '%U %G %a' /tmp/test.txt")
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
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)),
					Check: resource.ComposeTestCheckFunc(
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
							if err != nil {
								return err
							}

							// check that the file was deleted
							out, err := sshClient.RunCommand(t.Context(), "ls /tmp/test.txt")
							if err == nil {
								return fmt.Errorf("file was not deleted")
							}

							if out != "ls: cannot access '/tmp/test.txt': No such file or directory\n" {
								return fmt.Errorf("unexpected output: %s", out)
							}

							return nil
						},
					),
				},
			},
		})
	})
}

func testProviderConfig(privateKey string, user string, host string, port string) string {
	return fmt.Sprintf(`
	provider "setup" {
		private_key = "%s"
		user        = "%s"
		host        = "%s"
		port        = "%s"
	}
		`, privateKey, user, host, port)
}

func testFileResourceConfig(path string, mode string, owner int, group int, content string) string {
	return fmt.Sprintf(`
resource "setup_file" "file" {
	path    = "%s"
	mode    = "%s"
	owner   = %d
	group   = %d
	content = "%s"
}
`, path, mode, owner, group, content)
}
