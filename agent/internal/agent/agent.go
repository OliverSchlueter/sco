package agent

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/OliverSchlueter/sco-agent/internal/runtime"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolserver"
)

type Agent struct {
	nodeName string
	server   *protocolserver.Server

	rt    runtime.Runtime
	tasks []runtime.TaskConfig
}

type Configuration struct {
	NodeName    string
	Endpoint    string
	AccessToken string
}

func NewAgent(cfg Configuration) (*Agent, error) {
	// runtime
	rt, err := runtime.NewDockerRuntime()
	if err != nil {
		return nil, fmt.Errorf("error initializing runtime: %w", err)
	}

	// protocol server
	cs := protocolcommandstore.New()
	agentServer := protocolserver.New("----", cs)
	if err := agentServer.ConnectTo(cfg.Endpoint); err != nil {
		return nil, fmt.Errorf("error connecting to server: %w", err)
	}
	time.Sleep(100 * time.Millisecond) // Wait for connection to establish

	a := &Agent{
		nodeName: cfg.NodeName,
		server:   agentServer,
		rt:       rt,
		tasks:    []runtime.TaskConfig{},
	}

	// ping and auth
	if pingSuccess, err := a.Ping(); err != nil || !pingSuccess {
		slog.Error("Error pinging server", "err", err)
		return nil, fmt.Errorf("error pinging server: %w", err)
	}
	if authSuccess, err := a.TokenAuth(cfg.AccessToken); err != nil || !authSuccess {
		slog.Error("Error authenticating with server", "err", err)
		return nil, fmt.Errorf("error authenticating with server: %w", err)
	}
	if checkAuthSuccess, err := a.CheckAuth(); err != nil || !checkAuthSuccess {
		slog.Error("Error checking authentication", "err", err)
		return nil, fmt.Errorf("error checking authentication: %w", err)
	}
	a.initPingLoop()

	// reconcile tasks
	a.initReconcileLoop()
	if err := a.reconcile(); err != nil {
		return nil, fmt.Errorf("error during initial reconcile: %w", err)
	}

	return a, nil
}

func (a *Agent) initPingLoop() {
	go func() {
		t := time.NewTicker(1 * time.Second)
		for range t.C {
			if pingSuccess, err := a.Ping(); err != nil || !pingSuccess {
				slog.Error("Error pinging server", "err", err)
			}
		}
	}()
}
