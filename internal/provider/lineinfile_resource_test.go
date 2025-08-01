package provider

import (
	"fmt"
	"os"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLineinfileResource(t *testing.T) {
	t.Run("Test lineinfile create, update and delete", func(t *testing.T) {
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

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
				"setup": providerserver.NewProtocol6WithError(NewProvider()()),
			},
			Steps: []resource.TestStep{
				// First create the file with initial content
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) +
						testFileResourceConfig("/tmp/test.conf", "644", 0, 0, "# Configuration file"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_file.file", "path", "/tmp/test.conf"),
					),
				},
				// Test adding a line to existing file
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) +
						testFileResourceConfig("/tmp/test.conf", "644", 0, 0, "# Configuration file") +
						testLineinfileResourceConfig("/tmp/test.conf", "database_host=localhost", ""),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_lineinfile.test", "path", "/tmp/test.conf"),
						resource.TestCheckResourceAttr("setup_lineinfile.test", "line", "database_host=localhost"),
					),
				},
				// Test adding another line
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) +
						testFileResourceConfig("/tmp/test.conf", "644", 0, 0, "# Configuration file") +
						testLineinfileResourceConfig("/tmp/test.conf", "database_host=localhost", "") +
						testLineinfileResourceConfigNamed("/tmp/test.conf", "database_port=5432", "", "test2"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_lineinfile.test", "path", "/tmp/test.conf"),
						resource.TestCheckResourceAttr("setup_lineinfile.test2", "path", "/tmp/test.conf"),
						resource.TestCheckResourceAttr("setup_lineinfile.test2", "line", "database_port=5432"),
					),
				},
				// Test replacing a line using regexp
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) +
						testFileResourceConfig("/tmp/test.conf", "644", 0, 0, "# Configuration file") +
						testLineinfileResourceConfig("/tmp/test.conf", "database_host=remote", "database_host=.*") +
						testLineinfileResourceConfigNamed("/tmp/test.conf", "database_port=5432", "", "test2"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_lineinfile.test", "line", "database_host=remote"),
						resource.TestCheckResourceAttr("setup_lineinfile.test", "regexp", "database_host=.*"),
					),
				},
			},
		})
	})

	t.Run("Test lineinfile delete preserves content", func(t *testing.T) {
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

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
				"setup": providerserver.NewProtocol6WithError(NewProvider()()),
			},
			Steps: []resource.TestStep{
				// Create file and add line
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) +
						testFileResourceConfig("/tmp/delete_test.conf", "644", 0, 0, "# Test file") +
						testLineinfileResourceConfig("/tmp/delete_test.conf", "keep_this_line", ""),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_lineinfile.test", "line", "keep_this_line"),
					),
				},
				// Remove lineinfile resource (but file content should remain)
				{
					Config: testProviderConfig(keyPath.Name(), "test", "localhost", fmt.Sprintf("%d", port)) +
						testFileResourceConfig("/tmp/delete_test.conf", "644", 0, 0, "# Test file"),
					Check: resource.ComposeTestCheckFunc(
						// File should still exist
						resource.TestCheckResourceAttr("setup_file.file", "path", "/tmp/delete_test.conf"),
					),
				},
			},
		})
	})
}

func testLineinfileResourceConfig(path, line, regexp string) string {
	return testLineinfileResourceConfigNamed(path, line, regexp, "test")
}

func testLineinfileResourceConfigNamed(path, line, regexp, name string) string {
	config := fmt.Sprintf(`
resource "setup_lineinfile" "%s" {
  path = "%s"
  line = "%s"`, name, path, line)

	if regexp != "" {
		config += fmt.Sprintf(`
  regexp = "%s"`, regexp)
	}

	config += `
}`

	return config
}
