package main

import (
	"log/slog"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommands"
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

	cs := protocolcommands.New()

	cs.RegisterHandler(67, func(cmd *protocol.Command) (*protocol.Response, error) {
		slog.Info("Received command", "id", cmd.ID, "data", string(cmd.Payload))
		return &protocol.Response{
			Code: protocol.StatusCodeOK,
		}, nil
	})
	cs.RegisterMiddleware(func(next protocolcommands.Handler) protocolcommands.Handler {
		return func(cmd *protocol.Command) (*protocol.Response, error) {
			slog.Info("Hello from middleware", "id", cmd.ID, "data", string(cmd.Payload))
			return next(cmd)
		}
	})

	response := cs.Execute(&protocol.Command{
		ID:      67,
		Payload: []byte("Hello, world!"),
	})
	slog.Info("Response", "code", response.Code, "payload", response.Payload)
}
