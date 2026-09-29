package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/docker/go-sdk/client"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	mclient "github.com/moby/moby/client"
)

type inspectionClient struct {
	client.SDKClient
	inspectContainer func(string) (mclient.ContainerInspectResult, error)
	inspectImage     func(string) (mclient.ImageInspectResult, error)
}

func (c *inspectionClient) ContainerInspect(_ context.Context, taskID string, _ mclient.ContainerInspectOptions) (mclient.ContainerInspectResult, error) {
	return c.inspectContainer(taskID)
}

func (c *inspectionClient) ImageInspect(_ context.Context, ref string, _ ...mclient.ImageInspectOption) (mclient.ImageInspectResult, error) {
	return c.inspectImage(ref)
}

func TestIsTaskImageCurrent(t *testing.T) {
	const tag = "nginx:latest"
	for _, tc := range []struct {
		name         string
		containerID  string
		currentImage bool
	}{
		{name: "unchanged image", containerID: "sha256:new", currentImage: true},
		{name: "new image under the same tag", containerID: "sha256:old", currentImage: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &DockerRuntime{client: &inspectionClient{
				inspectContainer: func(taskID string) (mclient.ContainerInspectResult, error) {
					if taskID != "nginx01" {
						t.Fatalf("unexpected task: %s", taskID)
					}
					return mclient.ContainerInspectResult{Container: container.InspectResponse{
						Image:  tc.containerID,
						Config: &container.Config{Image: tag},
					}}, nil
				},
				inspectImage: func(ref string) (mclient.ImageInspectResult, error) {
					if ref != tag {
						t.Fatalf("unexpected image reference: %s", ref)
					}
					return mclient.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:new"}}, nil
				},
			}}

			current, err := rt.IsTaskImageCurrent(context.Background(), "nginx01", tag)
			if err != nil {
				t.Fatal(err)
			}
			if current != tc.currentImage {
				t.Fatalf("got current=%t, want %t", current, tc.currentImage)
			}
		})
	}
}

func TestIsTaskImageCurrentChecksEachContainerSharingTag(t *testing.T) {
	const tag = "nginx:latest"
	containerImages := map[string]string{
		"nginx01": "sha256:old",
		"nginx02": "sha256:old",
	}
	rt := &DockerRuntime{client: &inspectionClient{
		inspectContainer: func(taskID string) (mclient.ContainerInspectResult, error) {
			return mclient.ContainerInspectResult{Container: container.InspectResponse{
				Image:  containerImages[taskID],
				Config: &container.Config{Image: tag},
			}}, nil
		},
		inspectImage: func(ref string) (mclient.ImageInspectResult, error) {
			if ref != tag {
				t.Fatalf("unexpected image reference: %s", ref)
			}
			return mclient.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:new"}}, nil
		},
	}}

	current, err := rt.IsTaskImageCurrent(context.Background(), "nginx01", tag)
	if err != nil || current {
		t.Fatalf("first old container: current=%t, err=%v", current, err)
	}
	containerImages["nginx01"] = "sha256:new"

	current, err = rt.IsTaskImageCurrent(context.Background(), "nginx02", tag)
	if err != nil || current {
		t.Fatalf("second old container: current=%t, err=%v", current, err)
	}
	current, err = rt.IsTaskImageCurrent(context.Background(), "nginx01", tag)
	if err != nil || !current {
		t.Fatalf("first updated container: current=%t, err=%v", current, err)
	}
}

func TestIsTaskImageCurrentInspectionErrors(t *testing.T) {
	inspectErr := errors.New("inspection failed")
	for _, tc := range []struct {
		name         string
		containerErr error
		imageErr     error
	}{
		{name: "container inspection failed", containerErr: inspectErr},
		{name: "container missing", containerErr: errdefs.ErrNotFound},
		{name: "image inspection failed", imageErr: inspectErr},
		{name: "image missing", imageErr: errdefs.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imageInspected := false
			rt := &DockerRuntime{client: &inspectionClient{
				inspectContainer: func(string) (mclient.ContainerInspectResult, error) {
					return mclient.ContainerInspectResult{Container: container.InspectResponse{Image: "sha256:old"}}, tc.containerErr
				},
				inspectImage: func(string) (mclient.ImageInspectResult, error) {
					imageInspected = true
					return mclient.ImageInspectResult{}, tc.imageErr
				},
			}}

			wantErr := tc.containerErr
			if wantErr == nil {
				wantErr = tc.imageErr
			}
			current, err := rt.IsTaskImageCurrent(context.Background(), "nginx01", "nginx:latest")
			if current || !errors.Is(err, wantErr) {
				t.Fatalf("got current=%t, err=%v; want false, %v", current, err, wantErr)
			}
			if imageInspected != (tc.containerErr == nil) {
				t.Fatalf("image inspection called=%t, want %t", imageInspected, tc.containerErr == nil)
			}
		})
	}
}

func TestGetTaskInfoInspectionErrors(t *testing.T) {
	inspectErr := errors.New("container not found: daemon unavailable")
	for _, tc := range []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "missing task", err: errdefs.ErrNotFound},
		{name: "wrapped missing task", err: fmt.Errorf("inspect container: %w", errdefs.ErrNotFound)},
		{name: "other error", err: inspectErr, wantErr: inspectErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &DockerRuntime{client: &inspectionClient{
				inspectContainer: func(string) (mclient.ContainerInspectResult, error) {
					return mclient.ContainerInspectResult{}, tc.err
				},
			}}

			info, err := rt.GetTaskInfo(context.Background(), "nginx01")
			if info != nil || !errors.Is(err, tc.wantErr) {
				t.Fatalf("got info=%v, err=%v; want nil, %v", info, err, tc.wantErr)
			}
		})
	}
}

type creationClient struct {
	client.SDKClient
	createContainer func(mclient.ContainerCreateOptions) (mclient.ContainerCreateResult, error)
}

func (c *creationClient) ContainerCreate(_ context.Context, opts mclient.ContainerCreateOptions) (mclient.ContainerCreateResult, error) {
	return c.createContainer(opts)
}

func TestCreateContainerVolumes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		volumes []string
	}{
		{name: "no configured volumes"},
		{name: "named volume", volumes: []string{"app-data:/data"}},
		{name: "host bind mount", volumes: []string{"/srv/app/data:/data"}},
		{name: "read-only bind and named volume", volumes: []string{"/srv/app/config:/etc/app:ro", "app-data:/data"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &DockerRuntime{client: &creationClient{
				createContainer: func(opts mclient.ContainerCreateOptions) (mclient.ContainerCreateResult, error) {
					if !reflect.DeepEqual(opts.HostConfig.Binds, tc.volumes) {
						t.Fatalf("Docker volume bindings = %v, want %v", opts.HostConfig.Binds, tc.volumes)
					}
					return mclient.ContainerCreateResult{ID: "container-id"}, nil
				},
			}}
			id, err := rt.createContainer(context.Background(), TaskConfig{Name: "app", Image: "app:latest", Volumes: tc.volumes})
			if err != nil || id != "container-id" {
				t.Fatalf("createContainer = %q, %v", id, err)
			}
		})
	}
}

func TestGetTaskInfoVolumes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		volumes []string
	}{
		{name: "no configured volumes"},
		{name: "named volume and read-only bind", volumes: []string{"app-data:/data", "/srv/app/config:/etc/app:ro"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &DockerRuntime{client: &inspectionClient{
				inspectContainer: func(string) (mclient.ContainerInspectResult, error) {
					return mclient.ContainerInspectResult{Container: container.InspectResponse{
						Config: &container.Config{
							Image: "app:latest",
							// Image-declared anonymous volumes are not task configuration.
							Volumes: map[string]struct{}{"/cache": {}},
						},
						HostConfig: &container.HostConfig{Binds: tc.volumes},
					}}, nil
				},
			}}
			info, err := rt.GetTaskInfo(context.Background(), "app")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(info.Volumes, tc.volumes) {
				t.Fatalf("inspected volumes = %v, want %v", info.Volumes, tc.volumes)
			}
			desired := TaskConfig{Name: "app", Image: "app:latest", Volumes: tc.volumes}
			if !desired.CompareTo(info) {
				t.Fatalf("inspected configuration %+v differs from desired %+v", info, desired)
			}
		})
	}
}
