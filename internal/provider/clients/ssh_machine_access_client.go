package clients

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

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

	// Ensure the file is readable by the SSH user for SCP access
	// We need to add read permissions that allow SCP to access the file
	// Add read permission for others to allow SCP access
	_, err = client.RunCommand(ctx, "sudo chmod o+r "+path)
	if err != nil {
		// If this fails, it's not critical, but ReadFile via SCP might fail
		tflog.Debug(ctx, "Warning: could not add read permission for SCP access: "+err.Error())
	}

	return nil
}

// ReadFile reads the content and metadata of a file from the remote machine using SCP
func (client *sshMachineAccessClient) ReadFile(ctx context.Context, path string) (FileInfo, error) {
	tflog.Debug(ctx, "Reading file from remote host using SCP: "+path)

	scpClient, err := scp.NewClientBySSH(client.Client)
	if err != nil {
		return FileInfo{}, fmt.Errorf("error creating SCP client: %w", err)
	}
	defer scpClient.Close()

	// Create a temporary file to store the downloaded content
	tmpFile, err := os.CreateTemp("", "readfile_*")
	if err != nil {
		return FileInfo{}, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	tflog.Debug(ctx, "Copying file from remote host to temp file: "+tmpFile.Name())

	// Copy file from remote host to local temp file
	err = scpClient.CopyFromRemote(ctx, tmpFile, path)
	if err != nil {
		// Check if it's a "No such file or directory" error
		if strings.Contains(err.Error(), "No such file or directory") {
			return FileInfo{}, FileNotFoundError{Path: path}
		}
		return FileInfo{}, fmt.Errorf("failed to copy file %s from remote host: %w", path, err)
	}

	// Read the content from the temp file
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return FileInfo{}, fmt.Errorf("failed to read temp file: %w", err)
	}

	// Get file metadata using stat command
	statOutput, err := client.RunCommand(ctx, fmt.Sprintf("stat -c '%%a %%U %%G' %s", path))
	if err != nil {
		return FileInfo{}, fmt.Errorf("failed to get file metadata: %w", err)
	}

	// Parse the stat output (mode owner group)
	parts := strings.Fields(strings.TrimSpace(statOutput))
	if len(parts) != 3 {
		return FileInfo{}, fmt.Errorf("unexpected stat output format: got %d parts, expected 3: %s", len(parts), statOutput)
	}

	tflog.Debug(ctx, "Successfully read file using SCP with metadata")
	return FileInfo{
		Content: string(content),
		Mode:    parts[0],
		Owner:   parts[1],
		Group:   parts[2],
	}, nil
}
