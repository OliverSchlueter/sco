package runtime

import (
	"context"
)

type Runtime interface {
	PullImage(ctx context.Context, image string) error

	StartTask(ctx context.Context, cfg TaskConfig) error
	StopTask(ctx context.Context, taskID string) error
	RemoveTask(ctx context.Context, taskID string) error

	GetTaskStatus(ctx context.Context, taskID string) (Status, error)
	GetTaskInfo(ctx context.Context, taskID string) (*TaskConfig, error)
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

// CompareTo compares two TaskConfig objects and returns true if they are equal, false otherwise.
func (t *TaskConfig) CompareTo(other *TaskConfig) bool {
	if t.Name != other.Name {
		return false
	}
	if t.Image != other.Image {
		return false
	}
	if t.MaxCPU != other.MaxCPU {
		return false
	}
	if t.MaxMemory != other.MaxMemory {
		return false
	}

	if len(t.ExposedPorts) != len(other.ExposedPorts) {
		return false
	}
	for k, v := range t.ExposedPorts {
		if other.ExposedPorts[k] != v {
			return false
		}
	}
	return true
}
