package runtime

import (
	"context"
)

type Runtime interface {
	PullImage(ctx context.Context, image string) error

	StartTask(ctx context.Context, cfg TaskConfig) error
	StopTask(ctx context.Context, taskID string) error

	GetTaskStatus(ctx context.Context, taskID string) (Status, error)
	ListTasks(ctx context.Context) (map[string]Status, error)
}

type TaskConfig struct {
	// Name is the container name.
	Name string

	// Image is the container image.
	Image string

	// ExposedPorts is a map of container port to host port.
	ExposedPorts map[string]string

	// MaxCPU is in cores.
	MaxCPU float32

	// MaxMemory is in MB.
	MaxMemory int64
}

type Status string

const (
	Running Status = "RUNNING"
	Stopped Status = "STOPPED"
	Unknown Status = "UNKNOWN"
)

type TaskStats struct {
}
