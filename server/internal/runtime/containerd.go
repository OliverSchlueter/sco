package runtime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
)

const namespace = "sco"

type ContainerdRuntime struct {
	client *containerd.Client
}

func NewContainerdRuntime(socket string) (*ContainerdRuntime, error) {
	client, err := containerd.New(socket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %w", err)
	}

	return &ContainerdRuntime{
		client: client,
	}, nil
}

func (r *ContainerdRuntime) withNamespace(ctx context.Context) context.Context {
	return namespaces.WithNamespace(ctx, namespace)
}

func (r *ContainerdRuntime) PullImage(ctx context.Context, image string) error {
	ctx = r.withNamespace(ctx)

	_, err := r.client.Pull(ctx, image, containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("pull image failed: %w", err)
	}

	return nil
}

func (r *ContainerdRuntime) StartTask(ctx context.Context, taskID string, image string) error {
	ctx = r.withNamespace(ctx)

	img, err := r.client.GetImage(ctx, image)
	if err != nil {
		return fmt.Errorf("get image failed: %w", err)
	}

	container, err := r.client.NewContainer(
		ctx,
		taskID,
		containerd.WithNewSnapshot(taskID+"-snapshot", img),
		containerd.WithNewSpec(oci.WithImageConfig(img)),
	)
	if err != nil {
		return fmt.Errorf("create container failed: %w", err)
	}

	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("create task failed: %w", err)
	}

	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("start task failed: %w", err)
	}

	go func() {
		statusC, err := task.Wait(ctx)
		if err != nil {
			return
		}

		status := <-statusC
		code, _, _ := status.Result()
		slog.Info("Task exited", slog.String("task_id", taskID), slog.String("status_code", fmt.Sprintf("%d", code)))

		task.Delete(ctx)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
	}()

	return nil
}

func (r *ContainerdRuntime) StopTask(ctx context.Context, taskID string) error {
	ctx = r.withNamespace(ctx)

	container, err := r.client.LoadContainer(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load container failed: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}

	// SIGTERM
	if err := task.Kill(ctx, 15); err != nil {
		return fmt.Errorf("kill task failed: %w", err)
	}

	_, err = task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("wait failed: %w", err)
	}

	if _, err := task.Delete(ctx); err != nil {
		return err
	}

	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return err
	}

	return nil
}

func (r *ContainerdRuntime) GetTaskStatus(ctx context.Context, taskID string) (Status, error) {
	ctx = r.withNamespace(ctx)

	container, err := r.client.LoadContainer(ctx, taskID)
	if err != nil {
		return Unknown, err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return Stopped, nil
	}

	status, err := task.Status(ctx)
	if err != nil {
		return Unknown, err
	}

	switch status.Status {
	case containerd.Running:
		return Running, nil
	case containerd.Stopped:
		return Stopped, nil
	default:
		return Unknown, nil
	}
}

func (r *ContainerdRuntime) ListTasks(ctx context.Context) (map[string]Status, error) {
	ctx = r.withNamespace(ctx)

	containers, err := r.client.Containers(ctx)
	if err != nil {
		return nil, err
	}

	var result = make(map[string]Status)

	for _, c := range containers {
		task, err := c.Task(ctx, nil)
		if err != nil {
			continue
		}

		status, err := task.Status(ctx)
		if err != nil {
			continue
		}

		switch status.Status {
		case containerd.Running:
			result[c.ID()] = Running
		case containerd.Stopped:
			result[c.ID()] = Stopped
		default:
			result[c.ID()] = Unknown
		}
	}

	return result, nil
}
