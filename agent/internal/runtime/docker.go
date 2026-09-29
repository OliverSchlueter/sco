package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/docker/go-sdk/client"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	mclient "github.com/moby/moby/client"
)

const scoServerLabel = "de.oliver.sco.task"

type DockerRuntime struct {
	client client.SDKClient
}

func NewDockerRuntime() (*DockerRuntime, error) {
	cli, err := client.New(context.Background())
	if err != nil {
		return nil, err
	}

	return &DockerRuntime{
		client: cli,
	}, nil
}

func (r *DockerRuntime) PullImage(ctx context.Context, image string) error {
	slog.Debug("Pulling image", slog.String("image", image))
	resp, err := r.client.ImagePull(ctx, image, mclient.ImagePullOptions{})
	if err != nil {
		return err
	}

	// wait for the pull to complete
	if err := resp.Wait(ctx); err != nil {
		return err
	}

	slog.Debug("Pulled image", slog.String("image", image))
	return nil
}

func (r *DockerRuntime) StartTask(ctx context.Context, cfg TaskConfig) error {
	var ctrID string

	ctrSummary, err := r.client.FindContainerByName(ctx, cfg.Name)
	if err != nil {
		// container does not exist, create it
		slog.Debug(
			"Creating container",
			slog.String("name", cfg.Name),
			slog.String("image", cfg.Image),
		)

		ctrID, err = r.createContainer(ctx, cfg)
		if err != nil {
			return err
		}

		slog.Debug(
			"Created container",
			slog.String("container_id", ctrID),
			slog.String("name", cfg.Name),
			slog.String("image", cfg.Image),
		)
	} else {
		// container exists
		ctrID = ctrSummary.ID

		// check if the container is already running
		if ctrSummary.State == container.StateRunning {
			slog.Debug(
				"Container is already running",
				slog.String("container_id", ctrID),
				slog.String("name", cfg.Name),
				slog.String("image", cfg.Image),
			)
			return nil
		}
	}

	slog.Debug(
		"Starting container",
		slog.String("container_id", ctrID),
		slog.String("name", cfg.Name),
		slog.String("image", cfg.Image),
	)

	if err := r.startContainer(ctx, ctrID); err != nil {
		return err
	}

	slog.Debug(
		"Started container",
		slog.String("container_id", ctrID),
		slog.String("name", cfg.Name),
		slog.String("image", cfg.Image),
	)
	return nil
}

func (r *DockerRuntime) createContainer(ctx context.Context, cfg TaskConfig) (string, error) {
	cfgData, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}

	portBindings, err := convertPortBindings(cfg.ExposedPorts)
	if err != nil {
		return "", err
	}

	resp, err := r.client.ContainerCreate(ctx, mclient.ContainerCreateOptions{
		Name: cfg.Name,
		Config: &container.Config{
			Image: cfg.Image,
			Labels: map[string]string{
				scoServerLabel: string(cfgData),
			},
			Env: convertEnvVariables(cfg.EnvironmentVariables),
		},
		HostConfig: &container.HostConfig{
			PortBindings: portBindings,
			RestartPolicy: container.RestartPolicy{
				Name:              container.RestartPolicyDisabled,
				MaximumRetryCount: 0,
			},
			NanoCPUs: int64(cfg.MaxCPU * 1e9),
			Memory:   cfg.MaxMemory * 1024 * 1024,
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (r *DockerRuntime) startContainer(ctx context.Context, containerID string) error {
	_, err := r.client.ContainerStart(ctx, containerID, mclient.ContainerStartOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (r *DockerRuntime) StopTask(ctx context.Context, taskID string) error {
	_, err := r.client.ContainerStop(ctx, taskID, mclient.ContainerStopOptions{})
	if err != nil {
		return err
	}

	slog.Debug("Stopped container", slog.String("name", taskID))
	return nil
}

func (r *DockerRuntime) RemoveTask(ctx context.Context, taskID string) error {
	_, err := r.client.ContainerRemove(ctx, taskID, mclient.ContainerRemoveOptions{})
	if err != nil {
		return err
	}

	slog.Debug("Removed container", slog.String("name", taskID))
	return nil
}

func (r *DockerRuntime) GetTaskStatus(ctx context.Context, taskID string) (Status, error) {
	summary, err := r.client.FindContainerByName(ctx, taskID)
	if err != nil {
		if err.Error() == fmt.Sprintf("container %s not found", taskID) {
			return StatusUnknown, nil
		}
		return StatusUnknown, err
	}

	// check if the container is a sco task
	if _, exists := summary.Labels[scoServerLabel]; !exists {
		return StatusUnknown, nil
	}

	switch summary.State {
	case container.StateRunning:
		return StatusRunning, nil
	case container.StateExited:
		return StatusStopped, nil
	default:
		return StatusUnknown, nil
	}
}

func (r *DockerRuntime) ListTasks(ctx context.Context) (map[string]Status, error) {
	list, err := r.client.ContainerList(ctx, mclient.ContainerListOptions{
		Filters: make(mclient.Filters).Add("label", scoServerLabel),
	})
	if err != nil {
		return nil, err
	}

	var result = make(map[string]Status)
	for _, summary := range list.Items {
		var status Status
		switch summary.State {
		case container.StateRunning:
			status = StatusRunning
		case container.StateExited:
			status = StatusStopped
		default:
			status = StatusUnknown
		}

		result[summary.Names[0][1:]] = status
	}

	return result, nil
}

func (r *DockerRuntime) GetTaskInfo(ctx context.Context, taskID string) (*TaskConfig, error) {
	result, err := r.client.ContainerInspect(ctx, taskID, mclient.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	image := result.Container.Config.Image
	memory := result.Container.HostConfig.Memory / (1024 * 1024)
	cpu := float32(result.Container.HostConfig.NanoCPUs) / 1e9

	env := make(map[string]string)
	for _, e := range result.Container.Config.Env {
		parts := strings.Split(e, "=")
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	exposedPorts := make(map[string]string)
	for port, bindings := range result.Container.HostConfig.PortBindings {
		if len(bindings) > 0 {
			exposedPorts[strconv.Itoa(int(port.Num()))] = bindings[0].HostPort
		}
	}

	return &TaskConfig{
		Name:                 taskID,
		Image:                image,
		EnvironmentVariables: env,
		ExposedPorts:         exposedPorts,
		MaxCPU:               cpu,
		MaxMemory:            memory,
	}, nil
}

func (r *DockerRuntime) IsTaskImageCurrent(ctx context.Context, taskID, image string) (bool, error) {
	current, err := r.client.ContainerInspect(ctx, taskID, mclient.ContainerInspectOptions{})
	if err != nil {
		return false, err
	}

	desired, err := r.client.ImageInspect(ctx, image)
	if err != nil {
		return false, err
	}

	return current.Container.Image == desired.ID, nil
}

//func (r *DockerRuntime) GetStats(ctx context.Context, taskID string) error {
//	data, err := r.client.ContainerStats(ctx, taskID, mclient.ContainerStatsOptions{})
//	if err != nil {
//		return err
//	}
//
//	bytes, err := io.ReadAll(data.Body)
//	if err != nil {
//		return err
//	}
//	fmt.Printf("STATS: %s\n", bytes)
//
//	var resp container.StatsResponse
//	if err := json.Unmarshal(bytes, &resp); err != nil {
//		return err
//	}
//	fmt.Printf("STATS: %#v\n", resp)
//	return nil
//}

func convertEnvVariables(env map[string]string) []string {
	var result []string
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

func convertPortBindings(ports map[string]string) (map[network.Port][]network.PortBinding, error) {
	portBindings := map[network.Port][]network.PortBinding{}

	for ctrPort, hostPort := range ports {
		port, err := network.ParsePort(ctrPort + "/tcp")
		if err != nil {
			return nil, fmt.Errorf("could not parse port: %v", ctrPort)
		}

		binding := network.PortBinding{
			HostIP:   netip.MustParseAddr("127.0.0.1"),
			HostPort: hostPort,
		}

		portBindings[port] = []network.PortBinding{binding}
	}

	return portBindings, nil
}
