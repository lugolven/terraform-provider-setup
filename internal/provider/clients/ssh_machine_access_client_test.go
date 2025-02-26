package clients

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	retry "github.com/avast/retry-go"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

func TestSshRunCommand(t *testing.T) {
	expectedHelloOutput := "hello\n"

	t.Run("successful command execution with pricate key", func(t *testing.T) {
		// Arrange
		keyPath, err := os.CreateTemp("", "key")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name())

		if err := CreateSSHKey(t, keyPath.Name()); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name() + ".pub")

		stopServer, err := StartDockerSSHServer(t, keyPath.Name()+".pub", 2222)
		if err != nil {
			t.Fatal(err)
		}
		defer stopServer()

		var client MachineAccessClient

		err = retry.Do(func() error {
			var dialErr error
			client, dialErr = CreateSSHMachineAccessClientBuilder("test", "localhost", 2222).WithPrivateKeyPath(keyPath.Name()).Build()

			log.Println("Trying to dial...")

			return dialErr
		}, retry.Attempts(20), retry.Delay(10*time.Second))
		if err != nil {
			t.Fatal(err)
		}

		// Act
		output, err := client.RunCommand(ctx, "echo hello")

		// Assert
		if err != nil {
			t.Fatal(err)
		}

		if output != expectedHelloOutput {
			t.Fatalf("unexpected output: %s", output)
		}
	})

	t.Run("successful command execution with ssh agent", func(t *testing.T) {
		// Arrange
		keyPath, err := os.CreateTemp("", "key")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name())

		if err := CreateSSHKey(t, keyPath.Name()); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyPath.Name() + ".pub")

		socket, stopAgent := startSSHAgent(t, keyPath.Name())
		defer stopAgent()

		stopServer, err := StartDockerSSHServer(t, keyPath.Name()+".pub", 2222)
		if err != nil {
			t.Fatal(err)
		}
		defer stopServer()

		var client MachineAccessClient

		err = retry.Do(func() error {
			var dialErr error
			client, dialErr = CreateSSHMachineAccessClientBuilder("test", "localhost", 2222).WithAgent(socket).Build()

			log.Println("Trying to dial...")

			return dialErr
		}, retry.Attempts(20), retry.Delay(10*time.Second))

		if err != nil {
			t.Fatal(err)
		}
		// Act
		output, err := client.RunCommand(ctx, "echo hello")

		// Assert
		if err != nil {
			t.Fatal(err)
		}

		if output != expectedHelloOutput {
			t.Fatalf("unexpected output: %s", output)
		}
	})
}

func startSSHAgent(t *testing.T, keyPath string) (string, func()) {
	socket, err := os.MkdirTemp("/tmp", "socket")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// remove the socket file
	err = os.RemoveAll(socket)
	if err != nil {
		t.Fatalf("failed to remove socket file: %v", err)
	}

	cmd := exec.Command("ssh-agent", "-a", socket) // #nosec G204 - this is only used for testing

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to start ssh agent: %v\n%v", err, string(output))
	}

	t.Logf("output: %s", string(output))

	PIDLine := strings.Split(string(output), "\n")[1]
	PID := strings.Split(strings.Split(PIDLine, ";")[0], "=")[1]

	t.Logf("Started ssh agent with PID %s", PID)

	t.Logf("Adding key to ssh agent")

	cmd = exec.Command("ssh-add", keyPath)

	cmd.Env = append(os.Environ(), "SSH_AUTH_SOCK="+socket, "SSH_AGENT_PID="+PID)

	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to add key to ssh agent: %v\n%v", err, string(output))
	}

	return socket, func() {
		cmd := exec.Command("ssh-agent", "-k")

		cmd.Env = append(os.Environ(), "SSH_AGENT_PID="+PID)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to stop ssh agent: %v\n%v", err, string(output))
		}
	}
}

func CreateSSHKey(t *testing.T, keyPath string) error {
	t.Log("Creating ssh key")

	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		if err := os.Remove(keyPath); err != nil {
			return fmt.Errorf("failed to remove existing key file: %w", err)
		}
	}

	cmd := exec.CommandContext(ctx, "ssh-keygen", "-t", "rsa", "-b", "4096", "-f", keyPath, "-N", "")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create ssh key: %w\n%v", err, string(output))
	}

	pubKeyPath := keyPath + ".pub"
	// check if the public key exists
	if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("public key file not found: %w", err)
	}

	t.Logf("Created ssh key %s", keyPath)

	return nil
}

func StartDockerSSHServer(t *testing.T, authorizedKeysPath string, port int) (func(), error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	buildCtx := bytes.NewBuffer(nil)

	// Create build context
	if err := buildTar("../../../test/image/", buildCtx); err != nil {
		return nil, err
	}

	ctx := context.Background()
	imageName := "test/" + randomString(10)
	t.Logf("Building image %s", imageName)
	buildResponse, err := cli.ImageBuild(ctx, buildCtx, types.ImageBuildOptions{
		Tags:           []string{imageName},
		Dockerfile:     "Dockerfile",
		Remove:         true,
		SuppressOutput: false,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to build image: %w", err)
	}

	defer buildResponse.Body.Close()

	_, err = io.Copy(os.Stdout, buildResponse.Body)
	if err != nil {
		log.Fatalf("Copy error: %v", err)
	}

	builtImage, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return nil, fmt.Errorf("image build failed: %s", err.Error())
	}

	t.Logf("Built image %s", builtImage.ID)

	t.Logf("Creating container from image %s", imageName)

	keyContent, err := os.ReadFile(authorizedKeysPath) // #nosec G304 - this is only used for testing
	if err != nil {
		return nil, fmt.Errorf("failed to read authorized_keys file: %w", err)
	}

	containerResponse, err := cli.ContainerCreate(ctx, &container.Config{
		Image: imageName,
		Cmd:   []string{string(keyContent)},
	}, &container.HostConfig{
		PortBindings: map[nat.Port][]nat.PortBinding{
			"22/tcp": {
				{
					HostIP:   "127.0.0.1",
					HostPort: fmt.Sprintf("%d", port),
				},
			},
		},
	}, nil, nil, "")

	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	t.Logf("Created container %s", containerResponse.ID)

	t.Logf("Starting container %s", containerResponse.ID)

	if err := cli.ContainerStart(ctx, containerResponse.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	t.Logf("Started container %s", containerResponse.ID)

	go func() {
		logs, err := cli.ContainerLogs(ctx, containerResponse.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			fmt.Printf("Copy error: %v", err)
		}
		defer logs.Close()

		if _, err := io.Copy(os.Stdout, logs); err != nil {
			fmt.Printf("Copy error: %v", err)
		}
	}()

	return func() {
		t.Logf("Stopping container %s", containerResponse.ID)

		if err := cli.ContainerStop(ctx, containerResponse.ID, container.StopOptions{}); err != nil {
			t.Fatalf("failed to stop container: %v", err)
		}

		t.Logf("Stopped container %s", containerResponse.ID)

		t.Logf("Removing container %s", containerResponse.ID)

		if err := cli.ContainerRemove(ctx, containerResponse.ID, container.RemoveOptions{}); err != nil {
			t.Fatalf("failed to remove container: %v", err)
		}

		t.Logf("Removed container %s", containerResponse.ID)

		t.Logf("Removing image %s", imageName)

		if _, err := cli.ImageRemove(ctx, imageName, types.ImageRemoveOptions{}); err != nil {
			t.Fatalf("failed to remove image: %v", err)
		}

		t.Logf("Removed image %s", imageName)
	}, nil
}

func randomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz")

	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))] // #nosec G404 - This is only used for testing
	}

	return string(s)
}

// from https://github.com/ubclaunchpad/inertia/blob/master/daemon/inertiad/build/util.go#L25
func buildTar(dir string, outputs ...io.Writer) error {
	// ensure the src actually exists before trying to tar it
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("Unable to tar files - %v", err.Error())
	}

	mw := io.MultiWriter(outputs...)

	gzw := gzip.NewWriter(mw)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
		// return on any error
		if err != nil {
			return err
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, dir, "", -1), string(filepath.Separator))

		// write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// return on non-regular files
		if !fi.Mode().IsRegular() {
			return nil
		}

		// open files for taring
		f, err := os.Open(file) // #nosec G304 - this is always reading the input directory
		if err != nil {
			return err
		}
		defer f.Close()

		// copy file data into tar writer
		_, err = io.Copy(tw, f)

		return err
	})
}
