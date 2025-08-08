package clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

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

func (client *localMachineAccessClient) CopyFile(ctx context.Context, localPath string, remotePath string) error {
	tflog.Debug(ctx, fmt.Sprintf("Copying file from %s to %s", localPath, remotePath))

	srcFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", localPath, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", remotePath, err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}
