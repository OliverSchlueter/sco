package main

import (
	"context"
	"log/slog"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-server/internal/runtime"
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

	test()
}

func test() {
	rt, err := runtime.NewDockerRuntime()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	if err := rt.PullImage(ctx, "nginx:latest"); err != nil {
		panic(err)
	}

	err = rt.StartTask(ctx, runtime.TaskConfig{
		Name:         "nginx01",
		Image:        "nginx:latest",
		ExposedPorts: map[string]string{"80": "8080"},
		MaxCPU:       0.5,
		MaxMemory:    200,
	})
	if err != nil {
		panic(err)
	}
}
