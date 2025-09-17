package provider

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestDockerImageLoadResource(t *testing.T) {
	t.Run("Test create and delete docker image load", func(t *testing.T) {
		setup := setupTestEnvironment(t)

		tempDir, err := os.MkdirTemp("", "docker-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		tarFile := filepath.Join(tempDir, "test-image.tar")
		if err := createTestDockerImageTar(tarFile); err != nil {
			t.Fatalf("Failed to create test tar file: %v", err)
		}

		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t),
					Check: resource.ComposeTestCheckFunc(
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							out, err := sshClient.RunCommand(context.Background(), "docker version")
							if err != nil {
								return fmt.Errorf("docker version failed %w, \nout:\n%s", err, out)
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t) + testDockerImageLoadResourceConfig(tarFile),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_docker_image_load.test", "tar_file", tarFile),
						resource.TestCheckResourceAttrSet("setup_docker_image_load.test", "image_sha"),
						func(s *terraform.State) error {
							rs, ok := s.RootModule().Resources["setup_docker_image_load.test"]
							if !ok {
								return fmt.Errorf("resource not found: setup_docker_image_load.test")
							}

							imageSHA := rs.Primary.Attributes["image_sha"]
							if imageSHA == "" {
								return fmt.Errorf("image_sha is empty")
							}

							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							_, err = sshClient.RunCommand(context.Background(), fmt.Sprintf("sudo docker inspect %s", imageSHA))
							if err != nil {
								return fmt.Errorf("docker image not found: %v", err)
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test update docker image load", func(t *testing.T) {
		setup := setupTestEnvironment(t)

		tempDir, err := os.MkdirTemp("", "docker-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		tarFile1 := filepath.Join(tempDir, "test-image1.tar")
		tarFile2 := filepath.Join(tempDir, "test-image2.tar")

		if err := createTestDockerImageTarWithContent(tarFile1, "first image content"); err != nil {
			t.Fatalf("Failed to create first test tar file: %v", err)
		}

		if err := createTestDockerImageTarWithContent(tarFile2, "second image content"); err != nil {
			t.Fatalf("Failed to create second test tar file: %v", err)
		}

		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t) + testDockerImageLoadResourceConfig(tarFile1),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_docker_image_load.test", "tar_file", tarFile1),
						resource.TestCheckResourceAttrSet("setup_docker_image_load.test", "image_sha"),
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t) + testDockerImageLoadResourceConfig(tarFile2),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_docker_image_load.test", "tar_file", tarFile2),
						resource.TestCheckResourceAttrSet("setup_docker_image_load.test", "image_sha"),
					),
				},
			},
		})
	})

	t.Run("Test tar file content change detection", func(t *testing.T) {
		setup := setupTestEnvironment(t)

		tempDir, err := os.MkdirTemp("", "docker-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		tarFile := filepath.Join(tempDir, "test-image.tar")

		// Create initial tar with first content
		if err := createTestDockerImageTarWithContent(tarFile, "initial content"); err != nil {
			t.Fatalf("Failed to create initial test tar file: %v", err)
		}

		var initialImageSHA string

		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t) + testDockerImageLoadResourceConfig(tarFile),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_docker_image_load.test", "tar_file", tarFile),
						resource.TestCheckResourceAttrSet("setup_docker_image_load.test", "image_sha"),
						func(s *terraform.State) error {
							rs, ok := s.RootModule().Resources["setup_docker_image_load.test"]
							if !ok {
								return fmt.Errorf("resource not found: setup_docker_image_load.test")
							}
							initialImageSHA = rs.Primary.Attributes["image_sha"]
							return nil
						},
					),
				},
				{
					PreConfig: func() {
						// Replace tar file with different content but same path
						if err := createTestDockerImageTarWithContent(tarFile, "updated content"); err != nil {
							t.Fatalf("Failed to create updated test tar file: %v", err)
						}
					},
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t) + testDockerImageLoadResourceConfig(tarFile),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_docker_image_load.test", "tar_file", tarFile),
						resource.TestCheckResourceAttrSet("setup_docker_image_load.test", "image_sha"),
						func(s *terraform.State) error {
							rs, ok := s.RootModule().Resources["setup_docker_image_load.test"]
							if !ok {
								return fmt.Errorf("resource not found: setup_docker_image_load.test")
							}

							newImageSHA := rs.Primary.Attributes["image_sha"]
							if newImageSHA == initialImageSHA {
								return fmt.Errorf("image SHA should have changed when tar content changed, but stayed: %s", newImageSHA)
							}

							// Verify the new image exists
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							_, err = sshClient.RunCommand(context.Background(), fmt.Sprintf("sudo docker inspect %s", newImageSHA))
							if err != nil {
								return fmt.Errorf("new docker image not found: %v", err)
							}

							return nil
						},
					),
				},
			},
		})
	})
}

func TestGetImageContentHashFromLocalTar(t *testing.T) {
	t.Run("should extract correct content hash from valid tar file", func(t *testing.T) {
		// Arrange
		tempDir, err := os.MkdirTemp("", "tar-inspect-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		tarFile := filepath.Join(tempDir, "test-image.tar")
		if err := createTestDockerImageTarWithContent(tarFile, "test content"); err != nil {
			t.Fatalf("Failed to create test tar file: %v", err)
		}

		resource := &dockerImageLoadResource{}

		// Act
		contentHash, err := resource.getImageContentHashFromLocalTar(t.Context(), tarFile)

		// Assert
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if contentHash == "" {
			t.Error("Expected non-empty content hash")
		}

		if !strings.HasSuffix(contentHash, ".json") {
			t.Errorf("Expected content hash to end with '.json', got: %s", contentHash)
		}
	})

	t.Run("should produce different content hashes for different content", func(t *testing.T) {
		// Arrange
		tempDir, err := os.MkdirTemp("", "tar-inspect-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		tarFile1 := filepath.Join(tempDir, "test-image1.tar")
		tarFile2 := filepath.Join(tempDir, "test-image2.tar")

		if err := createTestDockerImageTarWithContent(tarFile1, "first content"); err != nil {
			t.Fatalf("Failed to create first test tar file: %v", err)
		}

		if err := createTestDockerImageTarWithContent(tarFile2, "second content"); err != nil {
			t.Fatalf("Failed to create second test tar file: %v", err)
		}

		resource := &dockerImageLoadResource{}

		// Act
		contentHash1, err1 := resource.getImageContentHashFromLocalTar(t.Context(), tarFile1)
		contentHash2, err2 := resource.getImageContentHashFromLocalTar(t.Context(), tarFile2)

		// Assert
		if err1 != nil {
			t.Fatalf("Expected no error for first tar, got: %v", err1)
		}

		if err2 != nil {
			t.Fatalf("Expected no error for second tar, got: %v", err2)
		}

		if contentHash1 == contentHash2 {
			t.Errorf("Expected different content hashes for different content, but got same: %s", contentHash1)
		}
	})

	t.Run("should produce same content hash for same content", func(t *testing.T) {
		// Arrange
		tempDir, err := os.MkdirTemp("", "tar-inspect-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		tarFile1 := filepath.Join(tempDir, "test-image1.tar")
		tarFile2 := filepath.Join(tempDir, "test-image2.tar")

		content := "identical content"
		if err := createTestDockerImageTarWithContent(tarFile1, content); err != nil {
			t.Fatalf("Failed to create first test tar file: %v", err)
		}

		if err := createTestDockerImageTarWithContent(tarFile2, content); err != nil {
			t.Fatalf("Failed to create second test tar file: %v", err)
		}

		resource := &dockerImageLoadResource{}

		// Act
		contentHash1, err1 := resource.getImageContentHashFromLocalTar(t.Context(), tarFile1)
		contentHash2, err2 := resource.getImageContentHashFromLocalTar(t.Context(), tarFile2)

		// Assert
		if err1 != nil {
			t.Fatalf("Expected no error for first tar, got: %v", err1)
		}

		if err2 != nil {
			t.Fatalf("Expected no error for second tar, got: %v", err2)
		}

		if contentHash1 != contentHash2 {
			t.Errorf("Expected same content hashes for identical content, but got different: %s vs %s", contentHash1, contentHash2)
		}
	})

	t.Run("should return error for non-existent file", func(t *testing.T) {
		// Arrange
		resource := &dockerImageLoadResource{}

		// Act
		_, err := resource.getImageContentHashFromLocalTar(t.Context(), "/nonexistent/file.tar")

		// Assert
		if err == nil {
			t.Error("Expected error for non-existent file, but got nil")
		}
	})

	t.Run("should return error for invalid tar file", func(t *testing.T) {
		// Arrange
		tempDir, err := os.MkdirTemp("", "invalid-tar-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		invalidTarFile := filepath.Join(tempDir, "invalid.tar")
		if err := os.WriteFile(invalidTarFile, []byte("not a tar file"), 0600); err != nil {
			t.Fatalf("Failed to create invalid tar file: %v", err)
		}

		resource := &dockerImageLoadResource{}

		// Act
		_, err = resource.getImageContentHashFromLocalTar(t.Context(), invalidTarFile)

		// Assert
		if err == nil {
			t.Error("Expected error for invalid tar file, but got nil")
		}
	})
}

func testDockerSetupConfig(t *testing.T) string {
	t.Helper()

	return ""
}

func testDockerImageLoadResourceConfig(tarFile string) string {
	return fmt.Sprintf(`
resource "setup_docker_image_load" "test" {
  tar_file = "%s"
}
`, tarFile)
}

func createTestDockerImageTar(tarFile string) error {
	return createTestDockerImageTarWithContent(tarFile, "test content")
}

func createTestDockerImageTarWithContent(tarFile, content string) error {
	cleanPath := filepath.Clean(tarFile)
	file, err := os.Create(cleanPath)

	if err != nil {
		return err
	}

	defer file.Close()

	tarWriter := tar.NewWriter(file)
	defer tarWriter.Close()

	// We'll create the manifest after computing the config filename
	// Store tarWriter for later use
	var configFileName string

	// Create a proper layer tar file first
	var layerBuf bytes.Buffer
	layerTarWriter := tar.NewWriter(&layerBuf)

	// Add a simple file to the layer
	testFileHeader := &tar.Header{
		Name: "test.txt",
		Size: int64(len(content)),
		Mode: 0644,
	}
	if err := layerTarWriter.WriteHeader(testFileHeader); err != nil {
		return err
	}

	if _, err := layerTarWriter.Write([]byte(content)); err != nil {
		return err
	}

	layerTarWriter.Close()

	// Calculate the actual SHA256 of the layer
	layerBytes := layerBuf.Bytes()
	layerHash := sha256.Sum256(layerBytes)
	layerDiffID := fmt.Sprintf("sha256:%x", layerHash)

	// Create the config with the correct diff_id
	config := fmt.Sprintf(`{
		"architecture": "amd64",
		"config": {
			"Env": ["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"],
			"Cmd": ["sh"]
		},
		"rootfs": {
			"type": "layers",
			"diff_ids": ["%s"]
		},
		"history": [
			{
				"created": "2023-01-01T00:00:00Z",
				"created_by": "test"
			}
		]
	}`, layerDiffID)

	// Calculate the SHA256 of the config to use as the config filename
	configBytes := []byte(config)
	configHash := sha256.Sum256(configBytes)
	configSHA := fmt.Sprintf("%x", configHash)
	configFileName = fmt.Sprintf("%s.json", configSHA)

	// Create and add manifest with the computed config filename
	manifest := fmt.Sprintf(`[{"Config":"%s","RepoTags":["test:latest"],"Layers":["test-layer.tar"]}]`, configFileName)
	if err := addFileToTar(tarWriter, "manifest.json", []byte(manifest)); err != nil {
		return err
	}

	// Add config after calculating diff_id
	if err := addFileToTar(tarWriter, configFileName, []byte(config)); err != nil {
		return err
	}

	if err := addFileToTar(tarWriter, "test-layer.tar", layerBytes); err != nil {
		return err
	}

	return nil
}

func addFileToTar(tarWriter *tar.Writer, filename string, data []byte) error {
	header := &tar.Header{
		Name: filename,
		Size: int64(len(data)),
		Mode: 0600,
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	if _, err := tarWriter.Write(data); err != nil {
		return err
	}

	return nil
}
