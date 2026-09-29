package main

import (
	"encoding/json"
	"log/slog"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolserver"
	"github.com/OliverSchlueter/sco-protocol/pkg/sharedmodels"
)

var accessToken string
var authenticatedKey = "authenticated"

var tasks map[string][]sharedmodels.NodeTask

func main() {
	// Setup logging
	logService := sloki.NewService(sloki.Configuration{
		URL:          "http://localhost:3100/loki/api/v1/push",
		Service:      "sco",
		ConsoleLevel: slog.LevelDebug,
		LokiLevel:    slog.LevelInfo,
		EnableLoki:   false,
		Handlers:     []sloki.LogHandler{},
	})
	slog.SetDefault(slog.New(logService))

	// load tasks
	tasks = map[string][]sharedmodels.NodeTask{
		"agent01": {
			{
				ContainerName: "sco-nginx",
				Image:         "nginx:latest",
				EnvironmentVariables: []string{
					"ENV_VAR_1=value1",
				},
				ExposedPorts: map[string]string{"80": "8071"},
				Volumes:      []string{},
				MaxCPU:       0.5,
				MaxMemory:    200,
			},
			{
				ContainerName: "sco-gitea",
				Image:         "docker.gitea.com/gitea:latest",
				EnvironmentVariables: []string{
					"ENV_VAR_1=value2",
				},
				ExposedPorts: map[string]string{
					"3000": "3000",
					"22":   "2222",
				},
				Volumes: []string{
					//"/Users/oliver/Desktop/gitea_data:/data",
				},
				MaxCPU:    0.5,
				MaxMemory: 200,
			},
		},
	}

	// server
	accessToken = "token" // TODO: use MustGetString
	startScoServer()
	slog.Info("SCO server started")

	c := make(chan struct{})
	<-c
}

func startScoServer() {
	// register commands
	cs := protocolcommandstore.New()
	if err := cs.RegisterHandler(protocol.ServerCommandPing, handlePing); err != nil {
		slog.Error("Failed to register handler", sloki.WrapError(err))
		panic(err)
	}
	if err := cs.RegisterHandler(protocol.ServerCommandTokenAuth, handleTokenAuth); err != nil {
		slog.Error("Failed to register handler", sloki.WrapError(err))
		panic(err)
	}
	if err := cs.RegisterHandler(protocol.ServerCommandCheckAuth, handleCheckAuth); err != nil {
		slog.Error("Failed to register handler", sloki.WrapError(err))
		panic(err)
	}
	if err := cs.RegisterHandler(protocol.ServerCommandGetTasks, handleGetTasks); err != nil {
		slog.Error("Failed to register handler", sloki.WrapError(err))
		panic(err)
	}

	// start server
	srv := protocolserver.New("8080", cs)
	go srv.Start()
}

func handlePing(ctx *protocolcommandstore.ConnCtx, msg *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
	return &protocol.Response{
		Code:    protocol.StatusCodeOK,
		Payload: []byte("pong"),
	}, nil
}

func handleTokenAuth(ctx *protocolcommandstore.ConnCtx, msg *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
	providedToken := string(cmd.Payload)
	if providedToken != accessToken {
		return &protocol.Response{
			Code:    protocol.StatusInvalidAccessToken,
			Payload: []byte("invalid access token"),
		}, nil
	}

	ctx.SetCustomData(authenticatedKey, true)

	return &protocol.Response{
		Code:    protocol.StatusCodeOK,
		Payload: []byte("access token validated"),
	}, nil
}

func handleCheckAuth(ctx *protocolcommandstore.ConnCtx, msg *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
	authenticated, ok := ctx.GetCustomData(authenticatedKey)
	if !ok || !authenticated.(bool) {
		return &protocol.Response{
			Code:    protocol.StatusNotAuthenticated,
			Payload: []byte("not authenticated"),
		}, nil
	}

	return &protocol.Response{
		Code:    protocol.StatusCodeOK,
		Payload: []byte("authenticated"),
	}, nil
}

func handleGetTasks(ctx *protocolcommandstore.ConnCtx, msg *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
	if !checkAuthenticated(ctx) {
		return &protocol.Response{
			Code:    protocol.StatusNotAuthenticated,
			Payload: []byte("not authenticated"),
		}, nil
	}

	nodeName := string(cmd.Payload)
	nodeTasks, ok := tasks[nodeName]
	if !ok {
		return &protocol.Response{
			Code:    protocol.StatusNodeNotFound,
			Payload: []byte("node not found"),
		}, nil
	}

	// Serialize tasks to JSON
	tasksJSON, err := json.Marshal(nodeTasks)
	if err != nil {
		return &protocol.Response{
			Code:    protocol.StatusInternalError,
			Payload: []byte("failed to serialize tasks"),
		}, nil
	}

	return &protocol.Response{
		Code:    protocol.StatusCodeOK,
		Payload: tasksJSON,
	}, nil

}

func checkAuthenticated(ctx *protocolcommandstore.ConnCtx) bool {
	authenticated, ok := ctx.GetCustomData(authenticatedKey)
	if !ok || !authenticated.(bool) {
		return false
	}
	return true
}
