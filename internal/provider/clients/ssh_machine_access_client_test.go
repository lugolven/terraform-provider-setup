package clients

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/avast/retry-go"
)

func TestSshRunCommand(t *testing.T) {
	expectedHelloOutput := "hello\n"

	t.Run("successful command execution with pricate key", func(t *testing.T) {
		// Arrange
		keyPath, err := os.CreateTemp("", "key")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name())

		if err := CreateSSHKey(t, keyPath.Name()); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name() + ".pub")

		port, stopServer, err := StartDockerSSHServer(t, keyPath.Name()+".pub")
		if err != nil {
			t.Fatal(err)
		}
		defer stopServer()

		client, err := CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithPrivateKeyPath(keyPath.Name()).Build(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		// Act
		output, err := client.RunCommand(ctx, "echo hello")

		// Assert
		if err != nil {
			t.Fatal(err)
		}

		if output != expectedHelloOutput {
			t.Fatalf("unexpected output: %s", output)
		}
	})

	t.Run("successful command execution with ssh agent", func(t *testing.T) {
		// Arrange
		keyPath, err := os.CreateTemp("", "key")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name())

		if err := CreateSSHKey(t, keyPath.Name()); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name() + ".pub")

		socket, stopAgent := StartSSHAgent(t, keyPath.Name())
		defer stopAgent()

		port, stopServer, err := StartDockerSSHServer(t, keyPath.Name()+".pub")
		if err != nil {
			t.Fatal(err)
		}
		defer stopServer()

		var client MachineAccessClient
		err = retry.Do(func() error {
			var dialErr error
			client, dialErr = CreateSSHMachineAccessClientBuilder("test", "localhost", port).WithAgent(socket).Build(t.Context())

			log.Println("Trying to dial...")

			return dialErr
		}, retry.Attempts(5), retry.Delay(1*time.Second))

		if err != nil {
			t.Fatal(err)
		}

		// Act
		output, err := client.RunCommand(ctx, "echo hello")

		// Assert
		if err != nil {
			t.Fatal(err)
		}

		if output != expectedHelloOutput {
			t.Fatalf("unexpected output: %s", output)
		}
	})
}
