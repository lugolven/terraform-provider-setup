package clients

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Demo test showing how callers can catch FileNotFoundError
func TestFileNotFoundErrorDemo(t *testing.T) {
	t.Run("Demonstration of catching FileNotFoundError from ReadFile", func(t *testing.T) {
		// Arrange
		testPath := "/nonexistent/file.txt"
		expectedErrorMessage := "file /nonexistent/file.txt does not exist"

		// Example function that simulates ReadFile returning FileNotFoundError
		simulateReadFile := func(path string) (string, error) {
			return "", FileNotFoundError{Path: path}
		}

		var fileNotFoundErr FileNotFoundError

		// Act
		content, err := simulateReadFile(testPath)

		// Assert
		require.Error(t, err)
		assert.Empty(t, content)

		// Check if it's specifically a FileNotFoundError
		isFileNotFoundError := errors.As(err, &fileNotFoundErr)
		assert.True(t, isFileNotFoundError, "Expected FileNotFoundError but got: %v", err)
		assert.Equal(t, testPath, fileNotFoundErr.Path)
		assert.Equal(t, expectedErrorMessage, fileNotFoundErr.Error())

		t.Log("Successfully caught FileNotFoundError for path:", fileNotFoundErr.Path)
	})

	t.Run("Wrapped FileNotFoundError can still be detected", func(t *testing.T) {
		// Arrange
		testPath := "/test/file"
		fileNotFoundError := FileNotFoundError{Path: testPath}
		wrappedErr := errors.New("failed to read file: " + fileNotFoundError.Error())
		properlyWrappedErr := errors.Join(errors.New("failed to read file"), fileNotFoundError)
		var fileNotFoundErr FileNotFoundError

		// Act
		canDetectImproperWrap := errors.As(wrappedErr, &fileNotFoundErr)
		canDetectProperWrap := errors.As(properlyWrappedErr, &fileNotFoundErr)

		// Assert
		// This would be a normal error, not detectable as FileNotFoundError
		assert.False(t, canDetectImproperWrap)

		// But if we properly wrap with errors.Join, it can be detected
		assert.True(t, canDetectProperWrap)
		assert.Equal(t, testPath, fileNotFoundErr.Path)
	})

	t.Run("FileNotFoundError vs other errors", func(t *testing.T) {
		tests := []struct {
			name         string
			err          error
			expectFileNF bool
			expectedPath string
		}{
			{
				name:         "Direct FileNotFoundError",
				err:          FileNotFoundError{Path: "/direct/path"},
				expectFileNF: true,
				expectedPath: "/direct/path",
			},
			{
				name:         "Generic error",
				err:          errors.New("some other error"),
				expectFileNF: false,
			},
			{
				name:         "ExitError",
				err:          ExitError{ExitCode: 1},
				expectFileNF: false,
			},
			{
				name:         "Properly wrapped FileNotFoundError",
				err:          errors.Join(errors.New("operation failed"), FileNotFoundError{Path: "/wrapped/path"}),
				expectFileNF: true,
				expectedPath: "/wrapped/path",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var fileNotFoundErr FileNotFoundError
				isFileNotFound := errors.As(tt.err, &fileNotFoundErr)

				assert.Equal(t, tt.expectFileNF, isFileNotFound)
				if tt.expectFileNF {
					assert.Equal(t, tt.expectedPath, fileNotFoundErr.Path)
				}
			})
		}
	})
}
