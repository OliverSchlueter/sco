package agent

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-agent/internal/runtime"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolserver"
)

type Agent struct {
	nodeName    string
	accessToken string
	server      *protocolserver.Server

	rt    runtime.Runtime
	tasks []runtime.TaskConfig
}

type Configuration struct {
	NodeName    string
	Endpoint    string
	AccessToken string
	Runtime     string
}

func NewAgent(cfg Configuration) (*Agent, error) {
	// runtime
	var rt runtime.Runtime
	var err error
	switch cfg.Runtime {
	case "docker":
		rt, err = runtime.NewDockerRuntime()
	default:
		return nil, fmt.Errorf("unsupported runtime: %s", cfg.Runtime)
	}
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
		nodeName:    cfg.NodeName,
		accessToken: cfg.AccessToken,
		server:      agentServer,
		rt:          rt,
		tasks:       []runtime.TaskConfig{},
	}

	// ping and auth
	if pingSuccess, err := a.Ping(); err != nil || !pingSuccess {
		return nil, fmt.Errorf("error pinging server: %w", err)
	}
	if err := a.authenticate(); err != nil {
		return nil, fmt.Errorf("error authenticating with server: %w", err)
	}
	a.initPingLoop()

	// reconcile tasks
	if err := a.reconcile(); err != nil {
		return nil, fmt.Errorf("error during initial reconcile: %w", err)
	}
	a.initReconcileLoop()

	return a, nil
}

func (a *Agent) initPingLoop() {
	go func() {
		t := time.NewTicker(1 * time.Second)
		for range t.C {
			if pingSuccess, err := a.Ping(); err != nil || !pingSuccess {
				slog.Error("Error pinging server", sloki.WrapError(err))
				continue
			}

			checkAuthSuccess, err := a.CheckAuth()
			if err != nil {
				slog.Error("Error checking authentication", sloki.WrapError(err))
				continue
			}
			if !checkAuthSuccess {
				if err := a.authenticate(); err != nil {
					slog.Error("Error re-authenticating with server", sloki.WrapError(err))
				}
				slog.Info("Re-authenticated with server")
				continue
			}
		}
	}()
}

func (a *Agent) authenticate() error {
	if authSuccess, err := a.TokenAuth(a.accessToken); err != nil || !authSuccess {
		return fmt.Errorf("error authenticating with server: %w", err)
	}
	if checkAuthSuccess, err := a.CheckAuth(); err != nil || !checkAuthSuccess {
		return fmt.Errorf("error checking authentication: %w", err)
	}
	return nil
}
