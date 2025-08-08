package clients

import (
	"context"
	"fmt"
	"net"
	"path/filepath"

	"os"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type sshMachineAccessClientBuilder struct {
	user string
	host string
	port int

	agent          *string
	privateKeyPath *string
}

// CreateSSHMachineAccessClientBuilder creates an SSH machine access client builder
func CreateSSHMachineAccessClientBuilder(user string, host string, port int) *sshMachineAccessClientBuilder {
	return &sshMachineAccessClientBuilder{
		user: user,
		host: host,
		port: port,
	}
}

func (builder *sshMachineAccessClientBuilder) WithAgent(agent string) *sshMachineAccessClientBuilder {
	builder.agent = &agent
	return builder
}

func (builder *sshMachineAccessClientBuilder) WithPrivateKeyPath(privateKeyPath string) *sshMachineAccessClientBuilder {
	builder.privateKeyPath = &privateKeyPath
	return builder
}

func (builder *sshMachineAccessClientBuilder) buildAuthMethod() ([]ssh.AuthMethod, error) {
	if builder.agent != nil && builder.privateKeyPath != nil {
		return nil, fmt.Errorf("only one of agent or privateKeyPath can be set")
	}

	if builder.agent != nil {
		sshAgent, err := net.Dial("unix", *builder.agent)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't connect to ssh-agent")
		}

		sshAgentClient := agent.NewClient(sshAgent)
		signers, err := sshAgentClient.Signers()

		if err != nil {
			return nil, errors.Wrap(err, "couldn't get signers from ssh-agent")
		}

		return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil
	}

	if builder.privateKeyPath != nil {
		publicKeyFile, err := publicKeyFile(*builder.privateKeyPath)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't load public key file")
		}

		return []ssh.AuthMethod{publicKeyFile}, nil
	}

	return nil, fmt.Errorf("either agent or privateKeyPath must be set")
}

// CreateSSHMachineAccessClient creates a new ssh machine access client.
func (builder *sshMachineAccessClientBuilder) Build(ctx context.Context) (MachineAccessClient, error) {
	auth, err := builder.buildAuthMethod()
	if err != nil {
		return nil, err
	}

	sshConfig := &ssh.ClientConfig{
		User:            builder.user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106 - todo: make this configurable
	}

	addr := fmt.Sprintf("%v:%v", builder.host, builder.port)
	tflog.Debug(ctx, "Dialing "+addr)
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
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return string(out), ExitError{
				ExitCode: exitErr.ExitStatus(),
			}
		}

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

func (client *sshMachineAccessClient) CopyFile(ctx context.Context, localPath string, remotePath string) error {
	scpClient, err := scp.NewClientBySSH(client.Client)
	if err != nil {
		return fmt.Errorf("error creating new SSH session from existing connection: %w", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Copying file from %s to %s", localPath, remotePath))

	f, err := os.Open(localPath) // #nosec G304
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer f.Close()

	err = scpClient.CopyFromFile(ctx, *f, remotePath, "0644")
	if err != nil {
		return fmt.Errorf("failed to copy file to remote host: %w", err)
	}

	return nil
}
