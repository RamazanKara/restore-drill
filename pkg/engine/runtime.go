package engine
package engine

import (
	"context"
	"io"
)

// Runtime abstracts the container execution environment.
type Runtime interface {
	// Create provisions a container with the given spec.
	Create(ctx context.Context, spec ContainerSpec) (Container, error)

	// Exec runs a command inside the container.
	Exec(ctx context.Context, c Container, cmd []string) ([]byte, error)

	// CopyTo copies data into the container filesystem.
	CopyTo(ctx context.Context, c Container, dest string, src io.Reader) error

	// Destroy tears down the container and cleans up.
	Destroy(ctx context.Context, c Container) error

	// Logs returns the container's log stream.
	Logs(ctx context.Context, c Container) (io.ReadCloser, error)
}

// Container represents a running ephemeral container.
type Container interface {
	// ID returns the container identifier.
	ID() string

	// Host returns the hostname or IP for connecting to the container.
	Host() string

	// Port returns the mapped port for the given container port.
	Port(containerPort int) int
}

// ContainerSpec defines what container to create.
type ContainerSpec struct {
	Image       string
	Env         map[string]string
	Ports       []int
	MemoryLimit int64
	CPULimit    int64
	Cmd         []string
}
