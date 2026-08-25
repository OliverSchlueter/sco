package gateway

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"

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
				g.startHttpServer(service)
			} else {
				return fmt.Errorf("unknown service type: %s", service.Type)
			}
		}
	}

	return nil
}

func (g *Gateway) startTcpServer(service *cluster.Service) {
	for ctrPort, hostPort := range service.Ports {
		go func(service *cluster.Service, ctrPort, hostPort string) {
			ln, err := net.Listen("tcp", ":"+hostPort)
			if err != nil {
				slog.Error(
					"Could not start TCP server",
					slog.String("service", service.Name),
					slog.String("port", hostPort),
					sloki.WrapError(err),
				)
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
						slog.Error("No endpoint found", slog.String("service", service.Name))
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
						slog.Error(
							"Could not forward TCP connection",
							slog.String("service", service.Name),
							slog.String("public_port", hostPort),
							slog.String("endpoint", endpoint.Address()),
							sloki.WrapError(err),
						)
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
		}(service, ctrPort, hostPort)
	}
}

func (g *Gateway) startHttpServer(service *cluster.Service) {
	for ctrPort, hostPort := range service.Ports {
		go func(service *cluster.Service, ctrPort, hostPort string) {
			mux := http.NewServeMux()

			mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				endpoint := service.PickEndpoint(ctrPort)
				if endpoint == nil {
					slog.Error("No endpoint found", slog.String("service", service.Name))
					http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
					return
				}

				slog.Debug(
					"Forwarding HTTP request",
					slog.String("service", service.Name),
					slog.String("public_port", hostPort),
					slog.String("endpoint", endpoint.Address()),
				)

				resp, err := endpoint.ForwardHttpReq(r)
				if err != nil {
					slog.Error(
						"Could not forward HTTP request",
						slog.String("service", service.Name),
						slog.String("public_port", hostPort),
						slog.String("endpoint", endpoint.Address()),
						sloki.WrapError(err),
					)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}

				w.WriteHeader(resp.StatusCode)
				io.Copy(w, resp.Body)
				resp.Body.Close()

				slog.Debug(
					"Forwarded HTTP request",
					slog.String("service", service.Name),
					slog.String("public_port", hostPort),
					slog.String("endpoint", endpoint.Address()),
				)
			}))

			err := http.ListenAndServe(":"+hostPort, mux)
			if err != nil {
				slog.Error(
					"Could not start HTTP server",
					slog.String("service", service.Name),
					slog.String("port", hostPort),
					sloki.WrapError(err),
				)
				return
			}

			slog.Info(
				"Started public TCP server",
				slog.String("service", service.Name),
				slog.String("port", hostPort),
			)

			<-make(chan struct{})
		}(service, ctrPort, hostPort)
	}
}
