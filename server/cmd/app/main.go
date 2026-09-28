package main

import (
	"log/slog"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolserver"
)

var accessToken string
var authenticatedKey = "authenticated"

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

	accessToken = "token" // TODO: use MustGetString

	slog.Info("Starting SCO server")
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
