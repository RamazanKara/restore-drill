// Package docker implements the engine.Runtime interface using the Docker SDK.
package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/RamazanKara/restore-drill/pkg/engine"
	cerrdefs "github.com/containerd/errdefs"
	dockercontainer "github.com/docker/docker/api/types/container"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

// Runtime implements engine.Runtime using the Docker Engine API.
type Runtime struct {
	client *client.Client
}

// New creates a new Docker runtime that connects to the local Docker daemon.
func New() (*Runtime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker: failed to create client: %w", err)
	}
	return &Runtime{client: cli}, nil
}

// Ping verifies that the Docker daemon is reachable.
func (r *Runtime) Ping(ctx context.Context) error {
	if _, err := r.client.Ping(ctx); err != nil {
		return fmt.Errorf("docker: ping daemon: %w", err)
	}
	return nil
}

// dockerContainer implements engine.Container.
type dockerContainer struct {
	id      string
	host    string
	portMap map[int]int
}

func (c *dockerContainer) ID() string   { return c.id }
func (c *dockerContainer) Host() string { return c.host }
func (c *dockerContainer) Port(containerPort int) int {
	return c.portMap[containerPort]
}

// Create pulls the image when it is not already available locally, then starts a container.
func (r *Runtime) Create(ctx context.Context, spec engine.ContainerSpec) (engine.Container, error) {
	if err := r.ensureImage(ctx, spec.Image); err != nil {
		return nil, err
	}

	// Build port bindings
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}
	for _, p := range spec.Ports {
		port := nat.Port(fmt.Sprintf("%d/tcp", p))
		exposedPorts[port] = struct{}{}
		portBindings[port] = []nat.PortBinding{
			{HostIP: "127.0.0.1", HostPort: "0"}, // random available port
		}
	}

	// Environment variables
	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}

	// Container config
	containerConfig := &dockercontainer.Config{
		Image:        spec.Image,
		Env:          env,
		ExposedPorts: exposedPorts,
		Cmd:          spec.Cmd,
	}

	// Host config with resource limits
	hostConfig := &dockercontainer.HostConfig{
		PortBindings: portBindings,
		AutoRemove:   false,
	}
	if spec.MemoryLimit > 0 {
		hostConfig.Memory = spec.MemoryLimit
	}
	if spec.CPULimit > 0 {
		hostConfig.NanoCPUs = spec.CPULimit
	}

	slog.Debug("creating container", "image", spec.Image)
	resp, err := r.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("docker: create container: %w", err)
	}

	if err := r.client.ContainerStart(ctx, resp.ID, dockercontainer.StartOptions{}); err != nil {
		// Cleanup on failure
		_ = r.client.ContainerRemove(ctx, resp.ID, dockercontainer.RemoveOptions{Force: true})
		return nil, fmt.Errorf("docker: start container: %w", err)
	}

	// Inspect to get port mappings
	inspect, err := r.client.ContainerInspect(ctx, resp.ID)
	if err != nil {
		_ = r.client.ContainerRemove(ctx, resp.ID, dockercontainer.RemoveOptions{Force: true})
		return nil, fmt.Errorf("docker: inspect container: %w", err)
	}

	portMap := make(map[int]int)
	for _, p := range spec.Ports {
		port := nat.Port(fmt.Sprintf("%d/tcp", p))
		bindings := inspect.NetworkSettings.Ports[port]
		if len(bindings) > 0 {
			hostPort, _ := strconv.Atoi(bindings[0].HostPort)
			portMap[p] = hostPort
		}
	}

	dc := &dockerContainer{
		id:      resp.ID[:12],
		host:    "127.0.0.1",
		portMap: portMap,
	}

	slog.Info("container started", "id", dc.id, "ports", portMap)
	return dc, nil
}

func (r *Runtime) ensureImage(ctx context.Context, imageRef string) error {
	if _, err := r.client.ImageInspect(ctx, imageRef); err == nil {
		slog.Debug("using local image", "image", imageRef)
		return nil
	} else if !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("docker: inspect image %s: %w", imageRef, err)
	}

	slog.Debug("pulling image", "image", imageRef)
	reader, err := r.client.ImagePull(ctx, imageRef, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("docker: pull image %s: %w", imageRef, err)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		_ = reader.Close()
		return fmt.Errorf("docker: read image pull output for %s: %w", imageRef, err)
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("docker: close image pull stream for %s: %w", imageRef, err)
	}
	return nil
}

// Exec runs a command inside the container and returns combined output.
func (r *Runtime) Exec(ctx context.Context, c engine.Container, cmd []string) ([]byte, error) {
	dc := c.(*dockerContainer)

	execConfig := dockercontainer.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := r.client.ContainerExecCreate(ctx, dc.id, execConfig)
	if err != nil {
		return nil, fmt.Errorf("docker: exec create: %w", err)
	}

	attachResp, err := r.client.ContainerExecAttach(ctx, execResp.ID, dockercontainer.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: exec attach: %w", err)
	}
	defer attachResp.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, attachResp.Reader)
	if err != nil {
		return nil, fmt.Errorf("docker: exec read output: %w", err)
	}

	// Check exit code
	inspectResp, err := r.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("docker: exec inspect: %w", err)
	}
	if inspectResp.ExitCode != 0 {
		return stdout.Bytes(), fmt.Errorf("docker: exec exited with code %d: %s", inspectResp.ExitCode, stderr.String())
	}

	return stdout.Bytes(), nil
}

// CopyTo copies a tar stream into the container filesystem through exec.
//
// This intentionally avoids Docker's container archive upload API. The daemon
// archive endpoint has had multiple no-fixed-version Moby advisories, and
// restore-drill only needs a narrow "tar from stdin" copy path for its own
// ephemeral targets.
func (r *Runtime) CopyTo(ctx context.Context, c engine.Container, dest string, src io.Reader) error {
	dc := c.(*dockerContainer)

	execConfig := dockercontainer.ExecOptions{
		Cmd:          []string{"tar", "xf", "-", "-C", dest},
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := r.client.ContainerExecCreate(ctx, dc.id, execConfig)
	if err != nil {
		return fmt.Errorf("docker: copy exec create: %w", err)
	}

	attachResp, err := r.client.ContainerExecAttach(ctx, execResp.ID, dockercontainer.ExecAttachOptions{})
	if err != nil {
		return fmt.Errorf("docker: copy exec attach: %w", err)
	}
	defer attachResp.Close()

	var stdout, stderr bytes.Buffer
	readErrCh := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(&stdout, &stderr, attachResp.Reader)
		readErrCh <- err
	}()

	copyErr := copyAndCloseWrite(attachResp.Conn, src)
	readErr := <-readErrCh
	if copyErr != nil {
		return fmt.Errorf("docker: copy stream to target: %w", copyErr)
	}
	if readErr != nil {
		return fmt.Errorf("docker: copy read output: %w", readErr)
	}

	inspectResp, err := r.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return fmt.Errorf("docker: copy exec inspect: %w", err)
	}
	if inspectResp.ExitCode != 0 {
		return fmt.Errorf("docker: copy exited with code %d: %s", inspectResp.ExitCode, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type closeWriter interface {
	CloseWrite() error
}

func copyAndCloseWrite(w io.Writer, src io.Reader) error {
	_, copyErr := io.Copy(w, src)
	closeErr := closeWrite(w)
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func closeWrite(w io.Writer) error {
	if cw, ok := w.(closeWriter); ok {
		return cw.CloseWrite()
	}
	if closer, ok := w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Destroy stops and removes the container.
func (r *Runtime) Destroy(ctx context.Context, c engine.Container) error {
	dc := c.(*dockerContainer)
	slog.Info("destroying container", "id", dc.id)

	timeout := 10
	stopOpts := dockercontainer.StopOptions{Timeout: &timeout}
	_ = r.client.ContainerStop(ctx, dc.id, stopOpts)
	return r.client.ContainerRemove(ctx, dc.id, dockercontainer.RemoveOptions{Force: true})
}
