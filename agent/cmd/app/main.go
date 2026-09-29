package main

import (
	"log/slog"
	"math/rand"

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

	agentServer := protocolserver.New("8081", protocolcommandstore.New())
	if err := agentServer.ConnectTo("localhost:8080"); err != nil {
		slog.Error("Error connecting to server", "err", err)
		return
	}

	// ping server
	pingSuccess, err := Ping(agentServer)
	if err != nil {
		slog.Error("Error pinging server", "err", err)
		return
	}
	if !pingSuccess {
		slog.Error("Ping failed")
		return
	}
	slog.Info("Ping successful")

	// authenticate with token
	authSuccess, err := TokenAuth(agentServer, "token")
	if err != nil {
		slog.Error("Error authenticating with server", "err", err)
		return
	}
	if !authSuccess {
		slog.Error("Authentication failed")
		return
	}
	slog.Info("Authentication successful")

	// confirm authentication
	checkAuthSuccess, err := CheckAuth(agentServer)
	if err != nil {
		slog.Error("Error checking authentication", "err", err)
		return
	}
	if !checkAuthSuccess {
		slog.Error("Check authentication failed")
		return
	}
	slog.Info("Check authentication successful")

	c := make(chan struct{})
	<-c
}

func Ping(srv *protocolserver.Server) (bool, error) {
	resp, err := srv.SendCmd(&protocol.Command{
		ReqID:   rand.Uint32(),
		ID:      protocol.ServerCommandPing,
		Payload: []byte("ping"),
	})
	if err != nil {
		return false, err
	}

	if resp.Code != protocol.StatusCodeOK {
		return false, nil
	}
	return true, nil
}

func TokenAuth(srv *protocolserver.Server, token string) (bool, error) {
	resp, err := srv.SendCmd(&protocol.Command{
		ReqID:   rand.Uint32(),
		ID:      protocol.ServerCommandTokenAuth,
		Payload: []byte(token),
	})
	if err != nil {
		return false, err
	}

	if resp.Code != protocol.StatusCodeOK {
		return false, nil
	}
	return true, nil
}

func CheckAuth(srv *protocolserver.Server) (bool, error) {
	resp, err := srv.SendCmd(&protocol.Command{
		ReqID:   rand.Uint32(),
		ID:      protocol.ServerCommandCheckAuth,
		Payload: []byte{},
	})
	if err != nil {
		return false, err
	}

	if resp.Code != protocol.StatusCodeOK {
		return false, nil
	}
	return true, nil
}
