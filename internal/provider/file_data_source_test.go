package provider

import (
	"context"
	"fmt"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestFileDataSource(t *testing.T) {
	const expectedContent = "hello\nworld\n"

	// Arrange - set up a single Docker container for all test cases
	setup := setupTestEnvironment(t)

	t.Run("Test read file", func(t *testing.T) {
		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					PreConfig: func() {
						// Create a test file before reading
						sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
						if err != nil {
							t.Fatal(err)
						}

						_, err = sshClient.RunCommand(context.Background(), "sudo sh -c 'echo -n \"hello\" > /tmp/test_read.txt && echo >> /tmp/test_read.txt && echo -n \"world\" >> /tmp/test_read.txt && echo >> /tmp/test_read.txt'")
						if err != nil {
							t.Fatalf("failed to create test file: %v", err)
						}

						_, err = sshClient.RunCommand(context.Background(), "sudo chmod 644 /tmp/test_read.txt")
						if err != nil {
							t.Fatalf("failed to chmod test file: %v", err)
						}
					},
					Config: testProviderConfig(setup, "test", "localhost") + testFileDataSourceConfig("/tmp/test_read.txt"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("data.setup_file.test", "path", "/tmp/test_read.txt"),
						resource.TestCheckResourceAttr("data.setup_file.test", "mode", "644"),
						resource.TestCheckResourceAttr("data.setup_file.test", "content", expectedContent),
					),
				},
			},
		})
	})

	t.Run("Test read file with stat info", func(t *testing.T) {
		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					PreConfig: func() {
						// Create a test file before reading
						sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
						if err != nil {
							t.Fatal(err)
						}

						_, err = sshClient.RunCommand(context.Background(), "sudo sh -c 'echo test-content > /tmp/test_read_stat.txt'")
						if err != nil {
							t.Fatalf("failed to create test file: %v", err)
						}
					},
					Config: testProviderConfig(setup, "test", "localhost") + testFileDataSourceConfig("/tmp/test_read_stat.txt"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("data.setup_file.test", "path", "/tmp/test_read_stat.txt"),
						resource.TestCheckResourceAttrSet("data.setup_file.test", "mode"),
						resource.TestCheckResourceAttrSet("data.setup_file.test", "owner"),
						resource.TestCheckResourceAttrSet("data.setup_file.test", "group"),
						resource.TestCheckResourceAttrSet("data.setup_file.test", "content"),
						resource.TestCheckResourceAttrSet("data.setup_file.test", "id"),
					),
				},
			},
		})
	})
}

func testFileDataSourceConfig(path string) string {
	return fmt.Sprintf(`
data "setup_file" "test" {
	path = "%s"
}
`, path)
}
