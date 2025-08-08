package provider

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t) + testDockerImageLoadResourceConfig(tarFile),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_docker_image_load.test", "tar_file", "/tmp/test-image.tar"),
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

		if err := createTestDockerImageTar(tarFile1); err != nil {
			t.Fatalf("Failed to create first test tar file: %v", err)
		}

		if err := createTestDockerImageTar(tarFile2); err != nil {
			t.Fatalf("Failed to create second test tar file: %v", err)
		}

		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t) + testDockerImageLoadResourceConfig(tarFile1),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_docker_image_load.test", "tar_file", "/tmp/test-image.tar"),
						resource.TestCheckResourceAttrSet("setup_docker_image_load.test", "image_sha"),
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testDockerSetupConfig(t) + testDockerImageLoadResourceConfig(tarFile2),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_docker_image_load.test", "tar_file", "/tmp/test-image.tar"),
						resource.TestCheckResourceAttrSet("setup_docker_image_load.test", "image_sha"),
					),
				},
			},
		})
	})
}

func testDockerSetupConfig(t *testing.T) string {
	t.Helper()
	// Download Docker GPG key dynamically
	// #nosec G107 - This is a test function using trusted Docker GPG key URL
	resp, err := http.Get("https://download.docker.com/linux/ubuntu/gpg")
	if err != nil {
		t.Fatalf("Failed to download Docker GPG key: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to download Docker GPG key: HTTP %d", resp.StatusCode)
	}

	keyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read Docker GPG key: %v", err)
	}

	dockerGpgKey := string(keyBytes)

	return fmt.Sprintf(`
resource "setup_apt_repository" "docker" {
  name = "docker"
  key  = <<EOT
%s
EOT
  url  = "https://download.docker.com/linux/ubuntu"
}

resource "setup_apt_packages" "docker_packages" {
  depends_on = [setup_apt_repository.docker]
  
  package {
    name = "docker-ce"
    absent = false
  }
}
`, dockerGpgKey)
}

func testDockerImageLoadResourceConfig(tarFile string) string {
	// Read the tar file content to embed it in the file resource
	// #nosec G304 - This is a test function reading test files created by the test
	content, err := os.ReadFile(tarFile)
	if err != nil {
		panic(fmt.Sprintf("Failed to read tar file: %v", err))
	}

	// Base64 encode the binary content for embedding in Terraform config
	encodedContent := base64.StdEncoding.EncodeToString(content)

	return fmt.Sprintf(`
resource "setup_file" "docker_tar" {
  depends_on = [setup_apt_packages.docker_packages]
  path = "/tmp/test-image.tar"
  mode = "644"
  owner = 0
  group = 0
  content = base64decode("%s")
}

resource "setup_docker_image_load" "test" {
  depends_on = [setup_file.docker_tar]
  tar_file = "/tmp/test-image.tar"
}
`, encodedContent)
}

func createTestDockerImageTar(tarFile string) error {
	cleanPath := filepath.Clean(tarFile)
	file, err := os.Create(cleanPath)

	if err != nil {
		return err
	}

	defer file.Close()

	tarWriter := tar.NewWriter(file)
	defer tarWriter.Close()

	manifest := `[{"Config":"test-config.json","RepoTags":["test:latest"],"Layers":["test-layer.tar"]}]`
	if err := addFileToTar(tarWriter, "manifest.json", []byte(manifest)); err != nil {
		return err
	}

	// Create a proper layer tar file first
	var layerBuf bytes.Buffer
	layerTarWriter := tar.NewWriter(&layerBuf)

	// Add a simple file to the layer
	testFileHeader := &tar.Header{
		Name: "test.txt",
		Size: 4,
		Mode: 0644,
	}
	if err := layerTarWriter.WriteHeader(testFileHeader); err != nil {
		return err
	}

	if _, err := layerTarWriter.Write([]byte("test")); err != nil {
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

	// Add config after calculating diff_id
	if err := addFileToTar(tarWriter, "test-config.json", []byte(config)); err != nil {
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
