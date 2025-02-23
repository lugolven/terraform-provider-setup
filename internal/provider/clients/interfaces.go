package clients

import "context"

type MachineAccessClient interface {
	RunCommand(ctx context.Context, command string) (string, error)
	WriteFile(ctx context.Context, path string, mode string, owner string, group string, content string) error
}
