package clients

import (
	"context"
	"fmt"
)

// FileInfo holds file content and metadata
type FileInfo struct {
	Content string
	Mode    string
	Owner   string
	Group   string
}

// MachineAccessClient defines how to interact with a machine.
type MachineAccessClient interface {
	RunCommand(ctx context.Context, command string) (string, error)
	WriteFile(ctx context.Context, path string, mode string, owner string, group string, content string) error
	ReadFile(ctx context.Context, path string) (FileInfo, error)
}

// ExitError describes an error that occurred during command execution.
type ExitError struct {
	ExitCode int
}

func (e ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.ExitCode)
}

// FileNotFoundError describes an error when a file does not exist.
type FileNotFoundError struct {
	Path string
}

func (e FileNotFoundError) Error() string {
	return fmt.Sprintf("file %s does not exist", e.Path)
}
