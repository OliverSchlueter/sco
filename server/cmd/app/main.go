package main

import (
	"log/slog"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolserver"
)

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

	startScoServer()

	c := make(chan struct{})
	<-c
}

func startScoServer() {
	cs := protocolcommandstore.New()
	err := cs.RegisterHandler(1, func(ctx *protocolcommandstore.ConnCtx, msg *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
		// echo back the payload
		return &protocol.Response{
			Code:    protocol.StatusCodeOK,
			Payload: cmd.Payload,
		}, nil
	})
	if err != nil {
		slog.Error("Failed to register handler", sloki.WrapError(err))
		panic(err)
	}
	srv := protocolserver.New("8080", cs)
	go srv.Start()
}
