package main

import (
	"log/slog"
	"time"

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

	slog.Info("Hello world")

	// starting sco-server
	cs := protocolcommandstore.New()
	cs.RegisterHandler(1, func(ctx *protocolcommandstore.ConnCtx, msg *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
		// echo back the payload
		return &protocol.Response{
			Code:    protocol.StatusCodeOK,
			Payload: cmd.Payload,
		}, nil
	})

	srv := protocolserver.New("8080", cs)
	go srv.Start()

	// starting sco-agent
	go func() {
		time.Sleep(1 * time.Second)

		agentServer := protocolserver.New("8081", protocolcommandstore.New())
		if err := agentServer.ConnectTo("localhost:8080"); err != nil {
			slog.Error("Error connecting to server", "err", err)
			return
		}

		resp, err := agentServer.SendCmd(&protocol.Command{
			ReqID:   420,
			ID:      1,
			Payload: []byte("Moin Meister"),
		})
		if err != nil {
			slog.Error("Error sending command", "err", err)
			return
		}
		slog.Info("Response", "resp_code", resp.Code, "resp_payload", string(resp.Payload))
	}()

	c := make(chan struct{})
	<-c
}
