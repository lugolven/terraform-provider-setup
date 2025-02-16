package provider

import (
	"fmt"
	"io/ioutil"
	"os"

	"golang.org/x/crypto/ssh"
)

func createSshClient(user string, publicKeyFilePath string) (*ssh.Client, error) {
	// todo: use public key authentication
	// tood: get those values from the provider configuration
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

	conn, err := ssh.Dial("tcp", "localhost:1234", ssh_config)

	return conn, err
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
