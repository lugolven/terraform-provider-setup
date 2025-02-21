package provider

import (
	"context"
	"fmt"

	"os"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/crypto/ssh"
)

func createSshClient(user string, publicKeyFilePath string, host string, port int) (*WrapperSshClient, error) {
	publicKeyFile, err := PublicKeyFile(publicKeyFilePath)
	if err != nil {
		pwd := os.Getenv("PWD")
		return nil, fmt.Errorf("failed to load public key file %s/%s: %w", pwd, publicKeyFilePath, err)
	}
	ssh_config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			publicKeyFile,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%v:%v", host, port)
	conn, err := ssh.Dial("tcp", addr, ssh_config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}
	return &WrapperSshClient{conn}, nil
}

func PublicKeyFile(file string) (ssh.AuthMethod, error) {
	buffer, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(key), nil
}

// todo: add abstraction remote command to potentially be able to run it with other protocols than ssh, like calling it from the host machine

type WrapperSshClient struct {
	*ssh.Client
}

func (client *WrapperSshClient) RunCommand(ctx context.Context, command string) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	tflog.Debug(ctx, "Running command: "+command)
	return session.CombinedOutput(command)
}
