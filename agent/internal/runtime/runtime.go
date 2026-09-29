package runtime

import (
	"context"
)

// Runtime defines an interface for managing container lifecycle operations, including image management and task control.
type Runtime interface {

	// PullImage pulls the specified container image from the registry and ensures it is available locally.
	PullImage(ctx context.Context, image string) error

	// StartTask initializes and starts a container task based on the provided configuration.
	StartTask(ctx context.Context, cfg TaskConfig) error

	// StopTask stops a running task identified by its unique task ID.
	// Returns an error if the task cannot be stopped.
	StopTask(ctx context.Context, taskID string) error

	// RemoveTask removes a task identified by taskID.
	// Returns an error if the operation fails.
	RemoveTask(ctx context.Context, taskID string) error

	// GetTaskStatus retrieves the current status of the specified task based on its taskID.
	GetTaskStatus(ctx context.Context, taskID string) (Status, error)

	// GetTaskInfo returns nil without an error when the task does not exist.
	GetTaskInfo(ctx context.Context, taskID string) (*TaskConfig, error)

	// IsTaskImageCurrent compares the task's image with the image currently
	// available locally under the given reference. PullImage should be called
	// first to check for a newer image in the registry.
	IsTaskImageCurrent(ctx context.Context, taskID, image string) (bool, error)

	// ListTasks returns a map of task IDs to their status.
	ListTasks(ctx context.Context) (map[string]Status, error)
}

// TaskConfig represents the configuration for a containerized task.
type TaskConfig struct {
	// Name is the container name.
	Name string

	// Image is the container image.
	Image string

	// EnvironmentVariables is a map of environment variables to set in the container.
	EnvironmentVariables map[string]string

	// ExposedPorts is a map of container port to host port.
	ExposedPorts map[string]string

	// MaxCPU is in cores.
	MaxCPU float32

	// MaxMemory is in MB.
	MaxMemory int64
}

type Status string

const (
	StatusRunning Status = "RUNNING"
	StatusStopped Status = "STOPPED"
	StatusUnknown Status = "UNKNOWN"
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

	for k, v := range t.EnvironmentVariables {
		if other.EnvironmentVariables[k] != v {
			return false
		}
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
