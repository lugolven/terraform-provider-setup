package provider

import (
	"fmt"
	"os"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// TestSetup represents the common test setup for all provider tests
type TestSetup struct {
	KeyPath    string
	Port       int
	StopServer func()
}

// setupTestEnvironment creates a common test environment with SSH keys and Docker server
func setupTestEnvironment(t *testing.T) *TestSetup {
	t.Helper()

	// Create temporary key file
	keyFile, err := os.CreateTemp("", "key")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := keyFile.Name()
	keyFile.Close()
	
	// Clean up key files when test finishes
	t.Cleanup(func() {
		os.Remove(keyPath)
		os.Remove(keyPath + ".pub")
	})

	// Create SSH key
	if err := clients.CreateSSHKey(t, keyPath); err != nil {
		t.Fatal(err)
	}

	// Start Docker SSH server
	port, stopServer, err := clients.StartDockerSSHServer(t, keyPath+".pub", keyPath)
	if err != nil {
		t.Fatal(err)
	}
	
	// Clean up server when test finishes
	t.Cleanup(stopServer)

	return &TestSetup{
		KeyPath:    keyPath,
		Port:       port,
		StopServer: stopServer,
	}
}

// getTestProviderFactories returns the standard provider factories for tests
func getTestProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"setup": providerserver.NewProtocol6WithError(NewProvider()()),
	}
}

func testProviderConfig(args ...interface{}) string {
	// New signature: testProviderConfig(setup *TestSetup, user string, host string)
	if len(args) == 3 {
		if setup, ok := args[0].(*TestSetup); ok {
			user := args[1].(string)
			host := args[2].(string)
			return fmt.Sprintf(`
	provider "setup" {
		private_key = "%s"
		user        = "%s"
		host        = "%s"
		port        = "%d"
	}
		`, setup.KeyPath, user, host, setup.Port)
		}
	}

	// Old signature: testProviderConfig(privateKey string, user string, host string, port string)
	if len(args) == 4 {
		privateKey := args[0].(string)
		user := args[1].(string)
		host := args[2].(string)
		port := args[3].(string)
		return fmt.Sprintf(`
	provider "setup" {
		private_key = "%s"
		user        = "%s"
		host        = "%s"
		port        = "%s"
	}
		`, privateKey, user, host, port)
	}

	panic("testProviderConfig: invalid number of arguments")
}
