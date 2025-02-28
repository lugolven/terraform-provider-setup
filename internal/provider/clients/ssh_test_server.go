package clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "embed"

	"github.com/avast/retry-go"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

//go:embed test_server.tar
var testServerTar []byte

func StartSSHAgent(t *testing.T, keyPath string) (string, func()) {
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

	// wait

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

	cmd := exec.Command("ssh-keygen", "-t", "rsa", "-b", "4096", "-f", keyPath, "-N", "")

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

func StartDockerSSHServer(t *testing.T, authorizedKeysPath string) (port int, stop func(), err error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return -1, nil, err
	}

	buildCtx := bytes.NewBuffer(testServerTar)

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
		return -1, nil, fmt.Errorf("failed to build image: %w", err)
	}

	defer buildResponse.Body.Close()
	// read buildResponse.Body until EOF
	for {
		_, err := buildResponse.Body.Read(make([]byte, 1024))
		if err == io.EOF {
			break
		}

		if err != nil {
			return -1, nil, fmt.Errorf("failed to read build response: %w", err)
		}
	}

	// _, err = io.Copy(os.Stdout, buildResponse.Body)
	// if err != nil {
	// 	log.Fatalf("Copy error: %v", err)
	// }

	keyContent, err := os.ReadFile(authorizedKeysPath) // #nosec G304 - this is only used for testing
	if err != nil {
		return -1, nil, fmt.Errorf("failed to read authorized_keys file: %w", err)
	}

	port, err = getFreePort()
	if err != nil {
		return -1, nil, fmt.Errorf("failed to get free port: %w", err)
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
		return -1, nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := cli.ContainerStart(ctx, containerResponse.ID, container.StartOptions{}); err != nil {
		return -1, nil, fmt.Errorf("failed to start container: %w", err)
	}

	err = retry.Do(func() error {
		conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
		if err != nil {
			return err
		}

		conn.Close()

		return nil
	}, retry.Attempts(60), retry.Delay(1*time.Second))

	if err != nil {
		return -1, nil, fmt.Errorf("failed to connect to container: %w", err)
	}

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

		prefixer := NewPrefixer(os.Stdout, func() string {
			return fmt.Sprintf("[%s] ", imageName)
		})
		defer prefixer.EnsureNewline()

		if _, err := io.Copy(prefixer, logs); err != nil {
			fmt.Printf("Copy error: %v", err)
		}
	}()

	return port, func() {

		if err := cli.ContainerStop(ctx, containerResponse.ID, container.StopOptions{}); err != nil {
			t.Fatalf("failed to stop container: %v", err)
		}

		if err := cli.ContainerRemove(ctx, containerResponse.ID, container.RemoveOptions{}); err != nil {
			t.Fatalf("failed to remove container: %v", err)
		}

		if _, err := cli.ImageRemove(ctx, imageName, types.ImageRemoveOptions{}); err != nil {
			t.Fatalf("failed to remove image: %v", err)
		}
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

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}

	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}

type Prefixer struct {
	prefixFunc      func() string
	writer          io.Writer
	trailingNewline bool
	buf             bytes.Buffer // reuse buffer to save allocations
}

// New creates a new Prefixer that forwards all calls to Write() to writer.Write() with all lines prefixed with the
// return value of prefixFunc. Having a function instead of a static prefix allows to print timestamps or other changing
// information.
func NewPrefixer(writer io.Writer, prefixFunc func() string) *Prefixer {
	return &Prefixer{prefixFunc: prefixFunc, writer: writer, trailingNewline: true}
}

func (pf *Prefixer) Write(payload []byte) (int, error) {
	pf.buf.Reset() // clear the buffer

	for _, b := range payload {
		if pf.trailingNewline {
			pf.buf.WriteString(pf.prefixFunc())
			pf.trailingNewline = false
		}

		pf.buf.WriteByte(b)

		if b == '\n' {
			// do not print the prefix right after the newline character as this might
			// be the very last character of the stream and we want to avoid a trailing prefix.
			pf.trailingNewline = true
		}
	}

	n, err := pf.writer.Write(pf.buf.Bytes())
	if err != nil {
		// never return more than original length to satisfy io.Writer interface
		if n > len(payload) {
			n = len(payload)
		}
		return n, err
	}

	// return original length to satisfy io.Writer interface
	return len(payload), nil
}

// EnsureNewline prints a newline if the last character written wasn't a newline unless nothing has ever been written.
// The purpose of this method is to avoid ending the output in the middle of the line.
func (pf *Prefixer) EnsureNewline() {
	if !pf.trailingNewline {
		fmt.Fprintln(pf.writer)
	}
}
