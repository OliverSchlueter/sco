package main

import (
	"fmt"
	"log/slog"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-agent/internal/agent"
	"github.com/OliverSchlueter/sco-agent/internal/config"
)

var cfg config.Config

func main() {
	// Load configuration
	loadedCfg, err := config.LoadConfig("config.json")
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

	_, err = agent.NewAgent(agent.Configuration{
		Endpoint:    cfg.Endpoint,
		AccessToken: cfg.AccessToken,
		NodeName:    cfg.NodeName,
	})
	if err != nil {
		slog.Error("Error creating agent", sloki.WrapError(err))
		return
	}

	slog.Info("Agent created successfully")

	c := make(chan struct{})
	<-c
}
