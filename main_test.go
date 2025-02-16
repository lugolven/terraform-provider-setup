package main

import (
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"
	"terraform-provider-setup/internal/provider"
	"testing"
	"time"

	retry "github.com/avast/retry-go"
	"golang.org/x/crypto/ssh"
)

func startTestServer() (*os.Process, error) {
	log.Println("Starting test server... ")
	// run a bash command to start the server
	cmd := exec.Command("make", "test-env")
	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	log.Printf("Test server started with pid %d", cmd.Process.Pid)

	return cmd.Process, nil
}

func TestHello(t *testing.T) {

	process, err := startTestServer()
	if err != nil {
		t.Errorf("Error starting test server %v", err)
	}
	defer func() {
		log.Println("SIGTERM test server...")
		process.Signal(syscall.SIGTERM)

		log.Println("Waiting for test server to exit...")
		process.Wait()

		log.Println("Test server exited.")
	}()

	// create an ssh client using golang.org/x/crypto/ssh
	log.Println("Creating ssh config...")
	publicKey, err := provider.PublicKeyFile(".ssh/id_rsa")
	if err != nil {
		t.Errorf("Failed to load public key: %s", err)
	}
	ssh_config := &ssh.ClientConfig{
		User: "test",
		Auth: []ssh.AuthMethod{
			publicKey,
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}

	log.Println("Dialing ssh server...")

	// todo: add a retry mechanism to handle connection errors gracefully
	var sshClient *ssh.Client
	err = retry.Do(func() error {
		var dialErr error
		sshClient, dialErr = ssh.Dial("tcp", "localhost:1234", ssh_config)
		log.Println("Trying to dial...")
		return dialErr
	}, retry.Attempts(20), retry.Delay(10*time.Second))
	if err != nil {
		t.Errorf("Failed to dial: %s", err)
	}
	log.Println("Connection established.")

	// echo hello using the ssh connection
	session, err := sshClient.NewSession()
	if err != nil {
		t.Errorf("Failed to create session: %s", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput("echo hello")
	if err != nil {
		t.Errorf("Failed to run: %s", err)
	}
	if string(out) != "hello\n" {
		t.Errorf("Unexpected output: %s", out)
	}
	log.Println("Output: ", string(out))

	err = sshClient.Close()
	if err != nil {
		t.Errorf("Failed to close connection: %s", err)
	}
	log.Println("Connection closed.")
}
