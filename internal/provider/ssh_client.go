package provider

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"golang.org/x/crypto/ssh"
)

func createSshClient(ctx context.Context, user string, publicKeyFilePath string, host string, port int) (*ssh.Client, error) {
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

// func that load a public key from a file
func PublicKeyFile(file string) (ssh.AuthMethod, error) {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(key), nil
}
