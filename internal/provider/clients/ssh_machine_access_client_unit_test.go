package clients

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileNotFoundError(t *testing.T) {
	// Arrange
	path := "/test/path"
	expectedMessage := "file /test/path does not exist"

	// Act
	err := FileNotFoundError{Path: path}
	actualMessage := err.Error()

	// Assert
	assert.Equal(t, expectedMessage, actualMessage)
}

func TestSSHMachineAccessClient_ReadFile_Unit(t *testing.T) {
	t.Run("ReadFile with SCP success", func(t *testing.T) {
		// This test requires a real SSH connection to test SCP functionality
		// It's more of an integration test, but kept here for completeness
		t.Skip("Requires real SSH connection - covered by integration tests")
	})

	t.Run("FileNotFoundError is typed correctly", func(t *testing.T) {
		// Arrange
		testPath := "/nonexistent/file.txt"
		err := FileNotFoundError{Path: testPath}
		var fnfErr FileNotFoundError

		// Act
		implementsError := func() bool {
			var _ error = err
			return true
		}()
		canBeTypeAsserted := errors.As(err, &fnfErr)

		// Assert
		assert.True(t, implementsError)
		assert.True(t, canBeTypeAsserted)
		assert.Equal(t, testPath, fnfErr.Path)
	})
}

func TestSSHMachineAccessClientBuilder(t *testing.T) {
	t.Run("CreateSSHMachineAccessClientBuilder", func(t *testing.T) {
		// Arrange
		expectedUser := "testuser"
		expectedHost := "testhost"
		expectedPort := 2222

		// Act
		builder := CreateSSHMachineAccessClientBuilder(expectedUser, expectedHost, expectedPort)

		// Assert
		assert.NotNil(t, builder)
		assert.Equal(t, expectedUser, builder.user)
		assert.Equal(t, expectedHost, builder.host)
		assert.Equal(t, expectedPort, builder.port)
		assert.Nil(t, builder.agent)
		assert.Nil(t, builder.privateKeyPath)
	})

	t.Run("WithAgent", func(t *testing.T) {
		// Arrange
		initialBuilder := CreateSSHMachineAccessClientBuilder("testuser", "testhost", 2222)
		expectedAgentPath := "/tmp/ssh-agent.sock"

		// Act
		builder := initialBuilder.WithAgent(expectedAgentPath)

		// Assert
		assert.NotNil(t, builder.agent)
		assert.Equal(t, expectedAgentPath, *builder.agent)
	})

	t.Run("WithPrivateKeyPath", func(t *testing.T) {
		// Arrange
		initialBuilder := CreateSSHMachineAccessClientBuilder("testuser", "testhost", 2222)
		expectedKeyPath := "/home/user/.ssh/id_rsa"

		// Act
		builder := initialBuilder.WithPrivateKeyPath(expectedKeyPath)

		// Assert
		assert.NotNil(t, builder.privateKeyPath)
		assert.Equal(t, expectedKeyPath, *builder.privateKeyPath)
	})

	t.Run("BuildAuthMethod with both agent and private key should fail", func(t *testing.T) {
		// Arrange
		builder := CreateSSHMachineAccessClientBuilder("testuser", "testhost", 2222)
		builder = builder.WithAgent("/tmp/ssh-agent.sock").WithPrivateKeyPath("/home/user/.ssh/id_rsa")
		expectedErrorMessage := "only one of agent or privateKeyPath can be set"

		// Act
		authMethods, err := builder.buildAuthMethod()

		// Assert
		assert.Error(t, err)
		assert.Nil(t, authMethods)
		assert.Contains(t, err.Error(), expectedErrorMessage)
	})

	t.Run("BuildAuthMethod with nonexistent private key should fail", func(t *testing.T) {
		// Arrange
		builder := CreateSSHMachineAccessClientBuilder("testuser", "testhost", 2222)
		nonexistentKeyPath := "/nonexistent/key"
		builder = builder.WithPrivateKeyPath(nonexistentKeyPath)

		// Act
		authMethods, err := builder.buildAuthMethod()

		// Assert
		assert.Error(t, err)
		assert.Nil(t, authMethods)
	})

	t.Run("BuildAuthMethod with no authentication should fail", func(t *testing.T) {
		// Arrange
		builder := CreateSSHMachineAccessClientBuilder("testuser", "testhost", 2222)
		expectedErrorMessage := "either agent or privateKeyPath must be set"

		// Act
		authMethods, err := builder.buildAuthMethod()

		// Assert
		assert.Error(t, err)
		assert.Nil(t, authMethods)
		assert.Contains(t, err.Error(), expectedErrorMessage)
	})
}

func TestSSHMachineAccessClient_Integration(t *testing.T) {
	// These tests require a real SSH server and are more integration tests
	// They are kept separate from unit tests

	t.Run("Integration tests", func(t *testing.T) {
		t.Skip("Integration tests are covered by the existing test files")
	})
}

// Mock test for ReadFile error handling
func TestReadFileErrorHandling(t *testing.T) {
	t.Run("FileNotFoundError can be detected by callers", func(t *testing.T) {
		// Arrange
		testPath := "/test/file.txt"
		var err error = FileNotFoundError{Path: testPath}
		var fileNotFoundErr FileNotFoundError

		// Act
		isFileNotFoundError := errors.As(err, &fileNotFoundErr)

		// Assert
		assert.True(t, isFileNotFoundError, "Expected FileNotFoundError but got different type")
		assert.Equal(t, testPath, fileNotFoundErr.Path)
	})

	t.Run("Other errors are not FileNotFoundError", func(t *testing.T) {
		// Arrange
		err := errors.New("some other error")
		var fileNotFoundErr FileNotFoundError

		// Act
		isFileNotFoundError := errors.As(err, &fileNotFoundErr)

		// Assert
		assert.False(t, isFileNotFoundError)
	})
}

// Test helper functions
func TestSSHKeyGeneration(t *testing.T) {
	t.Run("CreateSSHKey helper", func(t *testing.T) {
		// Arrange
		keyFile, err := os.CreateTemp("", "test_ssh_key")
		require.NoError(t, err)
		defer os.Remove(keyFile.Name())
		defer os.Remove(keyFile.Name() + ".pub")

		// Act
		err = CreateSSHKey(t, keyFile.Name())

		// Assert
		require.NoError(t, err)

		// Verify that both private and public keys were created
		_, err = os.Stat(keyFile.Name())
		assert.NoError(t, err, "Private key should exist")

		_, err = os.Stat(keyFile.Name() + ".pub")
		assert.NoError(t, err, "Public key should exist")
	})
}

// Test SSH client connection timeout behavior
func TestSSHConnectionTimeout(t *testing.T) {
	t.Run("Connection to invalid host should timeout", func(t *testing.T) {
		// Arrange
		builder := CreateSSHMachineAccessClientBuilder("testuser", "192.0.2.1", 22) // TEST-NET-1 (should not respond)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Act
		_, err := builder.Build(ctx)

		// Assert
		assert.Error(t, err)
		// The error should be related to connection timeout or unreachable host
	})
}

// Benchmark tests for performance
func BenchmarkSSHClientBuilder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		builder := CreateSSHMachineAccessClientBuilder("testuser", "testhost", 2222)
		builder = builder.WithPrivateKeyPath("/tmp/nonexistent")
		_, _ = builder.buildAuthMethod() // This will fail but we're measuring performance
	}
}
