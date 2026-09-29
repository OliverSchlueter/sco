package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/OliverSchlueter/sco-agent/internal/runtime"
)

func TestReconcileTask(t *testing.T) {
	cfg := runtime.TaskConfig{Name: "app", Image: "app:latest"}
	tests := []struct {
		name       string
		status     runtime.Status
		imageID    string
		newImageID string
		config     runtime.TaskConfig
		wantCalls  []string
	}{
		{
			name:   "unchanged running container",
			status: runtime.StatusRunning, imageID: "old", newImageID: "old", config: cfg,
			wantCalls: []string{"status app", "pull app:latest", "info app", "image app"},
		},
		{
			name:   "new image under the same tag",
			status: runtime.StatusRunning, imageID: "old", newImageID: "new", config: cfg,
			wantCalls: []string{"status app", "pull app:latest", "info app", "image app", "stop app", "remove app", "start app"},
		},
		{
			name:   "configuration changed",
			status: runtime.StatusRunning, imageID: "old", newImageID: "old",
			config:    runtime.TaskConfig{Name: "app", Image: "app:latest", MaxMemory: 128},
			wantCalls: []string{"status app", "pull app:latest", "info app", "image app", "stop app", "remove app", "start app"},
		},
		{
			name:   "new task",
			status: runtime.StatusUnknown, newImageID: "new",
			wantCalls: []string{"status app", "pull app:latest", "info app", "start app"},
		},
		{
			name:   "unchanged stopped container",
			status: runtime.StatusStopped, imageID: "old", newImageID: "old", config: cfg,
			wantCalls: []string{"status app", "pull app:latest", "info app", "image app", "start app"},
		},
		{
			name:   "stopped container with old image",
			status: runtime.StatusStopped, imageID: "old", newImageID: "new", config: cfg,
			wantCalls: []string{"status app", "pull app:latest", "info app", "image app", "remove app", "start app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newReconcileRuntime(cfg, tt.status, tt.imageID, tt.newImageID)
			if task := rt.tasks[cfg.Name]; task != nil {
				task.config = tt.config
			}
			a := &Agent{rt: rt}
			if err := a.reconcileTask(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rt.calls, tt.wantCalls) {
				t.Fatalf("calls = %v, want %v", rt.calls, tt.wantCalls)
			}
			assertCurrentTask(t, rt, cfg, tt.newImageID)
		})
	}
}

func TestReconcileTaskVolumeChanges(t *testing.T) {
	changes := []struct {
		name    string
		current []string
		desired []string
	}{
		{name: "adding a mount", desired: []string{"app-data:/var/lib/app"}},
		{
			name:    "changing a mount",
			current: []string{"app-data:/var/lib/app"},
			desired: []string{"app-data:/var/lib/app:ro"},
		},
		{name: "removing a mount", current: []string{"app-data:/var/lib/app"}},
	}
	for _, change := range changes {
		for _, status := range []runtime.Status{runtime.StatusRunning, runtime.StatusStopped} {
			t.Run(change.name+"/"+string(status), func(t *testing.T) {
				current := runtime.TaskConfig{Name: "app", Image: "app:latest", Volumes: change.current}
				desired := current
				desired.Volumes = change.desired
				rt := newReconcileRuntime(current, status, "current", "current")
				a := &Agent{rt: rt}
				if err := a.reconcileTask(context.Background(), desired); err != nil {
					t.Fatal(err)
				}

				wantCalls := []string{"status app", "pull app:latest", "info app", "image app"}
				if status == runtime.StatusRunning {
					wantCalls = append(wantCalls, "stop app")
				}
				wantCalls = append(wantCalls, "remove app", "start app")
				if !reflect.DeepEqual(rt.calls, wantCalls) {
					t.Fatalf("calls = %v, want %v", rt.calls, wantCalls)
				}
				assertCurrentTask(t, rt, desired, "current")
				if !reflect.DeepEqual(rt.tasks[desired.Name].config.Volumes, desired.Volumes) {
					t.Fatalf("volumes = %v, want %v", rt.tasks[desired.Name].config.Volumes, desired.Volumes)
				}

				rt.calls = nil
				if err := a.reconcileTask(context.Background(), desired); err != nil {
					t.Fatal(err)
				}
				wantCalls = []string{"status app", "pull app:latest", "info app", "image app"}
				if !reflect.DeepEqual(rt.calls, wantCalls) {
					t.Fatalf("updated container restarted again: calls = %v", rt.calls)
				}
			})
		}
	}
}

func TestReconcileTaskVolumeReordering(t *testing.T) {
	for _, status := range []runtime.Status{runtime.StatusRunning, runtime.StatusStopped} {
		t.Run(string(status), func(t *testing.T) {
			current := runtime.TaskConfig{
				Name:    "app",
				Image:   "app:latest",
				Volumes: []string{"app-data:/var/lib/app", "/etc/app:/etc/app:ro"},
			}
			desired := current
			desired.Volumes = []string{current.Volumes[1], current.Volumes[0]}
			rt := newReconcileRuntime(current, status, "current", "current")
			original := rt.tasks[current.Name]
			a := &Agent{rt: rt}
			if err := a.reconcileTask(context.Background(), desired); err != nil {
				t.Fatal(err)
			}
			wantCalls := []string{"status app", "pull app:latest", "info app", "image app"}
			if status == runtime.StatusStopped {
				wantCalls = append(wantCalls, "start app")
			}
			if !reflect.DeepEqual(rt.calls, wantCalls) {
				t.Fatalf("calls = %v, want %v", rt.calls, wantCalls)
			}
			if rt.tasks[current.Name] != original {
				t.Fatal("container was recreated after reordering volumes")
			}
			assertCurrentTask(t, rt, desired, "current")
		})
	}
}

func TestReconcileTaskFailuresPreserveContainer(t *testing.T) {
	cfg := runtime.TaskConfig{Name: "app", Image: "app:latest"}
	for _, operation := range []string{"status", "pull", "info", "image"} {
		t.Run(operation, func(t *testing.T) {
			rt := newReconcileRuntime(cfg, runtime.StatusRunning, "old", "new")
			// Pull must also precede destructive operations on configuration changes.
			if operation == "pull" {
				rt.tasks[cfg.Name].config.MaxMemory = 128
			}
			wantErr := errors.New("runtime failure")
			rt.failures[operation] = wantErr
			a := &Agent{rt: rt}
			if err := a.reconcileTask(context.Background(), cfg); !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			task := rt.tasks[cfg.Name]
			if task == nil || task.status != runtime.StatusRunning || task.imageID != "old" {
				t.Fatalf("existing container was changed after %s failure: %+v", operation, task)
			}
		})
	}
}

func TestReconcileTaskRetriesImageUpdate(t *testing.T) {
	cfg := runtime.TaskConfig{Name: "app", Image: "app:latest"}
	for _, operation := range []string{"stop", "remove", "start"} {
		t.Run(operation, func(t *testing.T) {
			rt := newReconcileRuntime(cfg, runtime.StatusRunning, "old", "new")
			wantErr := errors.New("restart failed")
			rt.failures[operation] = wantErr
			a := &Agent{rt: rt}
			if err := a.reconcileTask(context.Background(), cfg); !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if rt.images[cfg.Image] != "new" {
				t.Fatal("image was not pulled before restart")
			}

			delete(rt.failures, operation)
			if err := a.reconcileTask(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			assertCurrentTask(t, rt, cfg, "new")

			rt.calls = nil
			if err := a.reconcileTask(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			wantCalls := []string{"status app", "pull app:latest", "info app", "image app"}
			if !reflect.DeepEqual(rt.calls, wantCalls) {
				t.Fatalf("updated container restarted again: calls = %v", rt.calls)
			}
		})
	}
}

func TestReconcileTasksSharingImage(t *testing.T) {
	first := runtime.TaskConfig{Name: "app-1", Image: "app:latest"}
	second := runtime.TaskConfig{Name: "app-2", Image: "app:latest"}
	rt := newReconcileRuntime(first, runtime.StatusRunning, "old", "new")
	rt.tasks[second.Name] = &reconcileRuntimeTask{config: second, status: runtime.StatusRunning, imageID: "old"}
	a := &Agent{rt: rt}
	for _, cfg := range []runtime.TaskConfig{first, second} {
		if err := a.reconcileTask(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		assertCurrentTask(t, rt, cfg, "new")
	}
}

func assertCurrentTask(t *testing.T, rt *reconcileRuntime, cfg runtime.TaskConfig, imageID string) {
	t.Helper()
	task := rt.tasks[cfg.Name]
	if task == nil || task.status != runtime.StatusRunning || task.imageID != imageID || !cfg.CompareTo(&task.config) {
		t.Fatalf("task = %+v, want running with config %+v and image ID %q", task, cfg, imageID)
	}
}

type reconcileRuntimeTask struct {
	config  runtime.TaskConfig
	status  runtime.Status
	imageID string
}

type reconcileRuntime struct {
	tasks          map[string]*reconcileRuntimeTask
	images         map[string]string
	registryImages map[string]string
	failures       map[string]error
	calls          []string
}

func newReconcileRuntime(cfg runtime.TaskConfig, status runtime.Status, imageID, newImageID string) *reconcileRuntime {
	rt := &reconcileRuntime{
		tasks:          make(map[string]*reconcileRuntimeTask),
		images:         map[string]string{cfg.Image: imageID},
		registryImages: map[string]string{cfg.Image: newImageID},
		failures:       make(map[string]error),
	}
	if imageID != "" {
		rt.tasks[cfg.Name] = &reconcileRuntimeTask{config: cfg, status: status, imageID: imageID}
	}
	return rt
}

func (r *reconcileRuntime) record(operation, name string) error {
	r.calls = append(r.calls, operation+" "+name)
	return r.failures[operation]
}

func (r *reconcileRuntime) PullImage(_ context.Context, image string) error {
	if err := r.record("pull", image); err != nil {
		return err
	}
	r.images[image] = r.registryImages[image]
	return nil
}

func (r *reconcileRuntime) StartTask(_ context.Context, cfg runtime.TaskConfig) error {
	if err := r.record("start", cfg.Name); err != nil {
		return err
	}
	// Like Docker, starting an existing container retains its original image.
	if task := r.tasks[cfg.Name]; task != nil {
		task.status = runtime.StatusRunning
	} else {
		r.tasks[cfg.Name] = &reconcileRuntimeTask{config: cfg, status: runtime.StatusRunning, imageID: r.images[cfg.Image]}
	}
	return nil
}

func (r *reconcileRuntime) StopTask(_ context.Context, taskID string) error {
	if err := r.record("stop", taskID); err != nil {
		return err
	}
	r.tasks[taskID].status = runtime.StatusStopped
	return nil
}

func (r *reconcileRuntime) RemoveTask(_ context.Context, taskID string) error {
	if err := r.record("remove", taskID); err != nil {
		return err
	}
	delete(r.tasks, taskID)
	return nil
}

func (r *reconcileRuntime) GetTaskStatus(_ context.Context, taskID string) (runtime.Status, error) {
	if err := r.record("status", taskID); err != nil {
		return runtime.StatusUnknown, err
	}
	if task := r.tasks[taskID]; task != nil {
		return task.status, nil
	}
	return runtime.StatusUnknown, nil
}

func (r *reconcileRuntime) GetTaskInfo(_ context.Context, taskID string) (*runtime.TaskConfig, error) {
	if err := r.record("info", taskID); err != nil {
		return nil, err
	}
	if task := r.tasks[taskID]; task != nil {
		cfg := task.config
		return &cfg, nil
	}
	return nil, nil
}

func (r *reconcileRuntime) IsTaskImageCurrent(_ context.Context, taskID, image string) (bool, error) {
	if err := r.record("image", taskID); err != nil {
		return false, err
	}
	return r.tasks[taskID].imageID == r.images[image], nil
}

func (r *reconcileRuntime) ListTasks(_ context.Context) (map[string]runtime.Status, error) {
	result := make(map[string]runtime.Status)
	for name, task := range r.tasks {
		result[name] = task.status
	}
	return result, nil
}
