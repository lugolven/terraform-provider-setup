package clients

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
)

// MachineAccessClient defines how to interact with a machine.
type MachineAccessClient interface {
	RunCommand(ctx context.Context, command string) (string, error)
	WriteFile(ctx context.Context, path string, mode string, owner string, group string, content string) error
	CopyFile(ctx context.Context, localPath string, remotePath string) error
	GetDockerClient(ctx context.Context) (*client.Client, error)
}

// ExitError describes an error that occurred during command execution.
type ExitError struct {
	ExitCode int
}

func (e ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.ExitCode)
}
