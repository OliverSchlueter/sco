package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolserver"
	"github.com/OliverSchlueter/sco-protocol/pkg/sharedmodels"
)

var cfg Config
var authenticatedKey = "authenticated"

func main() {
	// Load configuration
	loadedCfg, err := LoadConfig("config.json")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		panic(err)
	}
	cfg = *loadedCfg

	// Setup logging
	logService := sloki.NewService(sloki.Configuration{
		URL:          "http://localhost:3100/loki/api/v1/push",
		Service:      "sco",
		ConsoleLevel: cfg.SlogLevel(),
		LokiLevel:    slog.LevelInfo,
		EnableLoki:   false,
		Handlers:     []sloki.LogHandler{},
	})
	slog.SetDefault(slog.New(logService))

	// server
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
	if providedToken != cfg.AccessToken {
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

	nodeTasks := []sharedmodels.NodeTask{}
	for _, t := range cfg.Tasks {
		if t.Node == nodeName {
			nodeTasks = append(nodeTasks, t)
		}
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
