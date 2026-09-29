package agent

import (
	"encoding/json"

	"github.com/OliverSchlueter/sco-agent/internal/runtime"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/sharedmodels"
)

func (a *Agent) Ping() (bool, error) {
	resp, err := a.server.SendCmd(&protocol.Command{
		ID:      protocol.ServerCommandPing,
		Payload: []byte("ping"),
	})
	if err != nil {
		return false, err
	}

	if resp.Code != protocol.StatusCodeOK {
		return false, nil
	}
	return true, nil
}

func (a *Agent) TokenAuth(token string) (bool, error) {
	resp, err := a.server.SendCmd(&protocol.Command{
		ID:      protocol.ServerCommandTokenAuth,
		Payload: []byte(token),
	})
	if err != nil {
		return false, err
	}

	if resp.Code != protocol.StatusCodeOK {
		return false, nil
	}
	return true, nil
}

func (a *Agent) CheckAuth() (bool, error) {
	resp, err := a.server.SendCmd(&protocol.Command{
		ID:      protocol.ServerCommandCheckAuth,
		Payload: []byte{},
	})
	if err != nil {
		return false, err
	}

	if resp.Code != protocol.StatusCodeOK {
		return false, nil
	}
	return true, nil
}

func (a *Agent) GetTasks() ([]runtime.TaskConfig, error) {
	resp, err := a.server.SendCmd(&protocol.Command{
		ID:      protocol.ServerCommandGetTasks,
		Payload: []byte(a.nodeName),
	})
	if err != nil {
		return nil, err
	}

	if resp.Code != protocol.StatusCodeOK {
		return nil, nil
	}

	var nts []sharedmodels.NodeTask
	if err := json.Unmarshal(resp.Payload, &nts); err != nil {
		return nil, err
	}

	// convert NodeTask to TaskConfig
	var tasks []runtime.TaskConfig
	for _, nt := range nts {
		tasks = append(tasks, runtime.TaskConfig{
			Name:                 nt.ContainerName,
			Image:                nt.Image,
			EnvironmentVariables: nt.EnvironmentVariables,
			ExposedPorts:         nt.ExposedPorts,
			MaxCPU:               nt.MaxCPU,
			MaxMemory:            nt.MaxMemory,
		})
	}

	return tasks, nil
}
