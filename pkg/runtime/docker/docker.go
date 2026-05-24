// Package docker implements the engine.Runtime interface using the Docker SDK.
package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
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
	containerConfig := &container.Config{
		Image:        spec.Image,
		Env:          env,
		ExposedPorts: exposedPorts,
		Cmd:          spec.Cmd,
	}

	// Host config with resource limits
	hostConfig := &container.HostConfig{
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

	if err := r.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Cleanup on failure
		_ = r.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return nil, fmt.Errorf("docker: start container: %w", err)
	}

	// Inspect to get port mappings
	inspect, err := r.client.ContainerInspect(ctx, resp.ID)
	if err != nil {
		_ = r.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
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

func (r *Runtime) ensureImage(ctx context.Context, image string) error {
	if _, _, err := r.client.ImageInspectWithRaw(ctx, image); err == nil {
		slog.Debug("using local image", "image", image)
		return nil
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("docker: inspect image %s: %w", image, err)
	}

	slog.Debug("pulling image", "image", image)
	reader, err := r.client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("docker: pull image %s: %w", image, err)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		_ = reader.Close()
		return fmt.Errorf("docker: read image pull output for %s: %w", image, err)
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("docker: close image pull stream for %s: %w", image, err)
	}
	return nil
}

// Exec runs a command inside the container and returns combined output.
func (r *Runtime) Exec(ctx context.Context, c engine.Container, cmd []string) ([]byte, error) {
	dc := c.(*dockerContainer)

	execConfig := types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := r.client.ContainerExecCreate(ctx, dc.id, execConfig)
	if err != nil {
		return nil, fmt.Errorf("docker: exec create: %w", err)
	}

	attachResp, err := r.client.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
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

// CopyTo copies data from a reader into the container filesystem.
func (r *Runtime) CopyTo(ctx context.Context, c engine.Container, dest string, src io.Reader) error {
	dc := c.(*dockerContainer)
	return r.client.CopyToContainer(ctx, dc.id, dest, src, types.CopyToContainerOptions{})
}

// Destroy stops and removes the container.
func (r *Runtime) Destroy(ctx context.Context, c engine.Container) error {
	dc := c.(*dockerContainer)
	slog.Info("destroying container", "id", dc.id)

	timeout := 10
	stopOpts := container.StopOptions{Timeout: &timeout}
	_ = r.client.ContainerStop(ctx, dc.id, stopOpts)
	return r.client.ContainerRemove(ctx, dc.id, container.RemoveOptions{Force: true})
}

// Logs returns the container's log output.
func (r *Runtime) Logs(ctx context.Context, c engine.Container) (io.ReadCloser, error) {
	dc := c.(*dockerContainer)
	return r.client.ContainerLogs(ctx, dc.id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
}

// WaitReady polls a TCP port until it accepts connections or the context expires.
func WaitReady(ctx context.Context, host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	slog.Debug("waiting for port", "addr", addr, "timeout", timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		dialer := net.Dialer{Timeout: 500 * time.Millisecond}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			slog.Debug("port ready", "addr", addr)
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for %s to be ready", addr)
}
