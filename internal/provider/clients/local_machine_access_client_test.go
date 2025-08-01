package clients

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalRunCommand(t *testing.T) {
	// Arrange
	client, err := CreateLocalMachineAccessClient()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("successful command execution", func(t *testing.T) {
		// Arrange
		expectedOutput := "hello\n"
		command := "echo hello"

		// Act
		output, err := client.RunCommand(t.Context(), command)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, output)
	})

	t.Run("failed command execution", func(t *testing.T) {
		// Arrange
		invalidCommand := "invalidcommand"

		// Act
		_, err := client.RunCommand(t.Context(), invalidCommand)

		// Assert
		assert.Error(t, err)
	})
}

func TestWriteFile(t *testing.T) {
	// Arrange
	client, err := CreateLocalMachineAccessClient()
	if err != nil {
		t.Fatal(err)
	}

	user, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	testFilePath := "/tmp/testfile"
	testContent := "test content"

	t.Run("successful file write", func(t *testing.T) {
		// Act
		err := client.WriteFile(t.Context(), testFilePath, "0644", user.Uid, user.Gid, testContent)
		if err != nil {
			t.Fatal(err)
		}

		// Assert
		content, err := os.ReadFile(testFilePath)

		assert.NoError(t, err)
		assert.Equal(t, testContent, string(content))

		fileInfo, err := os.Stat(testFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "-rw-r--r--", fileInfo.Mode().String())
		assert.Equal(t, user.Uid, strconv.FormatUint(uint64(fileInfo.Sys().(*syscall.Stat_t).Uid), 10))
		assert.Equal(t, user.Gid, strconv.FormatUint(uint64(fileInfo.Sys().(*syscall.Stat_t).Gid), 10))

		os.Remove(testFilePath)
	})
}
