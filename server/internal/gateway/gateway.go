package gateway

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-server/internal/cluster"
)

type Gateway struct {
	clusterStore *cluster.Store
}

func NewGateway(clusterStore *cluster.Store) *Gateway {
	return &Gateway{
		clusterStore: clusterStore,
	}
}

func (g *Gateway) StartPublicServers() error {
	for _, cl := range g.clusterStore.List() {
		for _, service := range cl.Services {
			if service.Type == cluster.ServiceTypeTCP {
				g.startTcpServer(service)
			} else if service.Type == cluster.ServiceTypeHTTP {
				// TODO start http server
			} else {
				return fmt.Errorf("unknown service type: %s", service.Type)
			}
		}
	}

	return nil
}

func (g *Gateway) startTcpServer(service *cluster.Service) {
	for ctrPort, hostPort := range service.Ports {
		go func() {
			ln, err := net.Listen("tcp", ":"+hostPort)
			if err != nil {
				slog.Error("Could not listen on port", slog.String("port", hostPort), sloki.WrapError(err))
				return
			}

			slog.Info(
				"Started public TCP server",
				slog.String("service", service.Name),
				slog.String("port", hostPort),
			)

			for {
				conn, err := ln.Accept()
				if err != nil {
					slog.Error("Could not accept connection", sloki.WrapError(err))
					return
				}
				go func(conn net.Conn) {

					endpoint := service.PickEndpoint(ctrPort)
					if endpoint == nil {
						slog.Error("No endpoint found for port", slog.String("port", ctrPort))
						conn.Close()
						return
					}

					slog.Debug(
						"Forwarding TCP connection",
						slog.String("service", service.Name),
						slog.String("public_port", hostPort),
						slog.String("endpoint", endpoint.Address()),
					)

					if err := endpoint.ForwardTcpConn(conn); err != nil {
						slog.Error("Could not forward connection", sloki.WrapError(err))
						conn.Close()
						return
					}

					slog.Debug(
						"Stopped forwarding TCP connection",
						slog.String("service", service.Name),
						slog.String("public_port", hostPort),
						slog.String("endpoint", endpoint.Address()),
					)
				}(conn)
			}
		}()
	}
}
