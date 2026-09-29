package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-agent/internal/runtime"
)

func (a *Agent) reconcile() error {
	tasks, err := a.GetTasks()
	if err != nil {
		return err
	}
	a.tasks = tasks

	ctx := context.Background()

	slog.Info("Reconciling tasks", "task_count", len(a.tasks))

	// stop tasks that are no longer in the task list
	runningTasks, err := a.rt.ListTasks(ctx)
	if err != nil {
		return err
	}
	for rt := range runningTasks {
		found := false
		for _, t := range a.tasks {
			if t.Name == rt {
				found = true
				break
			}
		}
		if !found {
			slog.Info("Stopping task that is no longer in the task list", "task", rt)
			if err := a.rt.StopTask(ctx, rt); err != nil {
				slog.Error("Error stopping task", "task", rt, sloki.WrapError(err))
				continue
			}
			if err := a.rt.RemoveTask(ctx, rt); err != nil {
				slog.Error("Error removing task", "task", rt, sloki.WrapError(err))
				continue
			}
			slog.Info("Task stopped and removed successfully", "task", rt)
		}
	}

	// reconcile tasks that are in the task list
	for _, t := range a.tasks {
		if err := a.reconcileTask(ctx, t); err != nil {
			slog.Error("Error reconciling task", "task", t.Name, sloki.WrapError(err))
			continue
		}
	}

	return nil
}

func (a *Agent) reconcileTask(ctx context.Context, t runtime.TaskConfig) error {
	status, err := a.rt.GetTaskStatus(ctx, t.Name)
	if err != nil {
		return err
	}

	// Pull before stopping any existing task so a failed pull leaves it running.
	if err := a.rt.PullImage(ctx, t.Image); err != nil {
		return err
	}

	current, err := a.rt.GetTaskInfo(ctx, t.Name)
	if err != nil {
		return err
	}

	restart := false
	if current != nil {
		imageCurrent, err := a.rt.IsTaskImageCurrent(ctx, t.Name, t.Image)
		if err != nil {
			return err
		}
		restart = !t.CompareTo(current) || !imageCurrent
		if restart {
			slog.Info("Task configuration or image has changed, restarting task", "task", t.Name)

			if status != runtime.StatusStopped {
				if err := a.rt.StopTask(ctx, t.Name); err != nil {
					return err
				}
			}
			if err := a.rt.RemoveTask(ctx, t.Name); err != nil {
				return err
			}
		} else if status == runtime.StatusRunning {
			slog.Info("Task is already running", "task", t.Name)
			return nil
		}
	}

	if err := a.rt.StartTask(ctx, t); err != nil {
		return err
	}

	if restart {
		slog.Info("Task restarted successfully", "task", t.Name)
	} else {
		slog.Info("Task started successfully", "task", t.Name)
	}

	return nil
}

func (a *Agent) initReconcileLoop() {
	go func() {
		for {
			time.Sleep(5 * time.Second)

			if err := a.reconcile(); err != nil {
				slog.Error("Error reconciling tasks", sloki.WrapError(err))
			}
		}
	}()
}
