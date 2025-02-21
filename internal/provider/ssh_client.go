package provider

import (
	"fmt"

	"os"

	"golang.org/x/crypto/ssh"
)

func createSshClient(user string, publicKeyFilePath string, host string, port int) (*ssh.Client, error) {
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
	return conn, nil
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

// todo: add a wrapper to run commands on the remote machine and handle sessions
// todo: add abstraction remote command to potentially be able to run it with other protocols than ssh, like calling it from the host machine
