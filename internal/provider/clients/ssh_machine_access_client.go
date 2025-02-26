package clients

import (
	"context"
	"fmt"
	"path/filepath"

	"os"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/crypto/ssh"
)

// CreateSSHMachineAccessClient creates a new ssh machine access client.
func CreateSSHMachineAccessClient(user string, publicKeyFilePath string, host string, port int) (MachineAccessClient, error) {
	publicKeyFile, err := publicKeyFile(publicKeyFilePath)
	if err != nil {
		pwd := os.Getenv("PWD")
		return nil, fmt.Errorf("failed to load public key file %s (pwd:%s): %w", publicKeyFilePath, pwd, err)
	}

	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			publicKeyFile,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106 - todo: make this configurable
	}

	addr := fmt.Sprintf("%v:%v", host, port)

	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	return &sshMachineAccessClient{conn}, nil
}

func publicKeyFile(file string) (ssh.AuthMethod, error) {
	// validate that the path is absolute
	if !filepath.IsAbs(file) {
		return nil, fmt.Errorf("public key file path must be absolute")
	}

	buffer, err := os.ReadFile(file) // #nosec 304 - todo: Find a way to make this secure
	if err != nil {
		return nil, fmt.Errorf("failed to read publicKeyFile: %w", err)
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse privateKey: %w", err)
	}

	return ssh.PublicKeys(key), nil
}

// todo: add abstraction remote command to potentially be able to run it with other protocols than ssh, like calling it from the host machine

type sshMachineAccessClient struct {
	*ssh.Client
}

func (client *sshMachineAccessClient) RunCommand(ctx context.Context, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	tflog.Debug(ctx, "Running command: "+command)
	out, err := session.CombinedOutput(command)

	if err != nil {
		return string(out), fmt.Errorf("failed to run command: %w", err)
	}

	return string(out), nil
}

func (client *sshMachineAccessClient) WriteFile(ctx context.Context, path string, mode string, owner string, group string, content string) error {
	scpClient, err := scp.NewClientBySSH(client.Client)
	if err != nil {
		return fmt.Errorf("error creating new SSH session from existing connection.\n %w", err)
	}

	// write file content to tmp file from the host
	tflog.Debug(ctx, "Writing file content to temp file")

	tmpFile, err := os.CreateTemp("", "tempfile")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	tflog.Debug(ctx, "Writing content to temp file "+tmpFile.Name())

	err = os.WriteFile(tmpFile.Name(), []byte(content), 0600)
	if err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	tflog.Debug(ctx, "Copying file to remote host "+path)

	// copy the file to the remote host
	f, _ := os.Open(tmpFile.Name())
	remoteTmpFile, _ := os.CreateTemp("", "tempfile")

	err = scpClient.CopyFromFile(ctx, *f, remoteTmpFile.Name(), "0700")
	if err != nil {
		return fmt.Errorf("failed to copy file to remote host: %w", err)
	}

	// move the file to the correct location
	_, err = client.RunCommand(ctx, "sudo mv "+remoteTmpFile.Name()+" "+path)
	if err != nil {
		return err
	}

	// set the owner and group of the remote file
	out, err := client.RunCommand(ctx, "sudo chown "+owner+":"+group+" "+path)
	if err != nil {
		return fmt.Errorf("failed to set owner and group: %s", out)
	}

	// set the mode of the remote file
	out, err = client.RunCommand(ctx, "sudo chmod "+mode+" "+path)
	if err != nil {
		return fmt.Errorf("failed to set mode: %s", out)
	}

	return nil
}
