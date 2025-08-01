package clients

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientConsistency(t *testing.T) {
	t.Run("Local client returns FileNotFoundError for nonexistent files", func(t *testing.T) {
		// Arrange
		client, err := CreateLocalMachineAccessClient()
		require.NoError(t, err)
		testPath := "/nonexistent/file/path"
		var fileNotFoundErr FileNotFoundError

		// Act
		_, err = client.ReadFile(context.Background(), testPath)

		// Assert
		require.Error(t, err)
		assert.True(t, errors.As(err, &fileNotFoundErr))
		assert.Equal(t, testPath, fileNotFoundErr.Path)
	})

}

// Integration test placeholder - would test SSH client consistency
func TestSSHClientFileNotFoundConsistency(t *testing.T) {
	t.Run("SSH client consistency", func(t *testing.T) {
		// Arrange
		// This would test that SSH client also returns FileNotFoundError
		// for nonexistent files, maintaining consistency with local client

		// Act & Assert
		t.Skip("Requires SSH server - covered by integration tests")
	})
}
