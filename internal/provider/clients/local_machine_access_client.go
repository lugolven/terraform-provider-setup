package clients

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type localMachineAccessClient struct {
}

// CreateLocalMachineAccessClient creates a new local machine access client.
func CreateLocalMachineAccessClient() (MachineAccessClient, error) {
	return &localMachineAccessClient{}, nil
}

func (client *localMachineAccessClient) RunCommand(_ context.Context, command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	if err != nil {
		return "", fmt.Errorf("failed to run command %s: %w", command, err)
	}

	return out.String(), nil
}

func (client *localMachineAccessClient) WriteFile(ctx context.Context, path string, mode string, owner string, group string, content string) error {
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

	tflog.Debug(ctx, "Moving file to actual path "+path)

	_, err = client.RunCommand(ctx, "mv "+tmpFile.Name()+" "+path)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, "Setting owner and group of the file")

	_, err = client.RunCommand(ctx, "chown "+owner+":"+group+" "+path)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, "Setting mode of the file")

	_, err = client.RunCommand(ctx, "chmod "+mode+" "+path)
	if err != nil {
		return err
	}

	return nil
}

// ReadFile reads the content and metadata of a file from the local machine
func (client *localMachineAccessClient) ReadFile(ctx context.Context, path string) (FileInfo, error) {
	tflog.Debug(ctx, "Reading file from local machine: "+path)

	content, err := os.ReadFile(path)
	if err != nil {
		// Check if it's a "no such file or directory" error
		if os.IsNotExist(err) {
			return FileInfo{}, FileNotFoundError{Path: path}
		}
		return FileInfo{}, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	// Get file metadata
	fileInfo, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, fmt.Errorf("failed to get file metadata: %w", err)
	}

	// Get file mode
	mode := fmt.Sprintf("%04o", fileInfo.Mode().Perm())

	// Get owner and group using stat syscall
	owner, group := "0", "0"
	if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		owner = fmt.Sprintf("%d", stat.Uid)
		group = fmt.Sprintf("%d", stat.Gid)
	} else {
		return FileInfo{}, fmt.Errorf("failed to get file owner/group information")
	}

	return FileInfo{
		Content: string(content),
		Mode:    mode,
		Owner:   owner,
		Group:   group,
	}, nil
}
