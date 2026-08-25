package main

import (
	"context"
	"log/slog"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-server/internal/cluster"
	"github.com/OliverSchlueter/sco-server/internal/gateway"
	"github.com/OliverSchlueter/sco-server/internal/node"
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

	testNode()
}

func testNode() {
	n := node.Node{
		ID:         "node-1",
		Host:       "localhost",
		MaxCPU:     14,
		UsedCPU:    4,
		MaxMemory:  24 * 1024 * 1024 * 1024,
		UsedMemory: 8 * 1024 * 1024 * 1024,
	}
	if err := n.Ping(); err != nil {
		panic(err)
	}
}

func testGateway() {
	cs := cluster.NewStore()

	// Add example cluster
	err := cs.Add(&cluster.Cluster{
		Name: "fun-cluster",
		Services: []*cluster.Service{
			//{
			//	Type:                cluster.ServiceTypeTCP,
			//	Name:                "tcp-service",
			//	Image:               "nginx:latest",
			//	Ports:               map[string]string{"80": "8080"},
			//	MaxCPU:              1,
			//	MaxMemory:           200,
			//	Replicas:            5,
			//	LoadBalanceStrategy: cluster.LoadBalanceStrategyRoundRobin,
			//	Endpoints: map[string][]*cluster.Endpoint{
			//		"80": {
			//			{
			//				NodeID: "node-1",
			//				Host:   "127.0.0.1",
			//				Port:   "8090",
			//			},
			//			{
			//				NodeID: "node-1",
			//				Host:   "127.0.0.1",
			//				Port:   "8091",
			//			},
			//		},
			//	},
			//},
			{
				Type:                cluster.ServiceTypeTCP,
				Name:                "http-service",
				Image:               "nginx:latest",
				Ports:               map[string]string{"80": "8080"},
				MaxCPU:              1,
				MaxMemory:           200,
				Replicas:            5,
				LoadBalanceStrategy: cluster.LoadBalanceStrategyRoundRobin,
				Endpoints: map[string][]*cluster.Endpoint{
					"80": {
						{
							NodeID: "node-1",
							Host:   "127.0.0.1",
							Port:   "8090",
						},
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	gw := gateway.NewGateway(cs)
	if err := gw.StartPublicServers(); err != nil {
		panic(err)
	}

	c := make(chan struct{})
	<-c
}

func testDockerRuntime() {
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
