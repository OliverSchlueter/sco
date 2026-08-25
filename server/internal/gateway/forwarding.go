package gateway

import (
	"io"
	"net"
	"net/http"

	"github.com/OliverSchlueter/sco-server/internal/cluster"
)

func (g *Gateway) forwardTcpConn(e *cluster.Endpoint, conn net.Conn) error {
	// TODO instead of creating a new connection, use one connection and multiplex it
	eConn, err := net.Dial("tcp", e.Address())
	if err != nil {
		return err
	}

	// Ensure both connections get closed
	defer conn.Close()
	defer eConn.Close()

	done := make(chan struct{}, 2)

	// client -> endpoint
	go func() {
		io.Copy(eConn, conn)
		done <- struct{}{}
	}()

	// endpoint -> client
	go func() {
		io.Copy(conn, eConn)
		done <- struct{}{}
	}()

	// Wait for one direction to finish
	<-done

	return nil
}

func (g *Gateway) forwardHttpReq(e *cluster.Endpoint, req *http.Request) (*http.Response, error) {
	req.URL.Host = e.Address()
	req.Host = e.Address()
	req.RemoteAddr = e.Address()
	req.Header.Set("X-Forwarded-For", e.Address())
	req.Header.Set("X-Forwarded-Host", req.Host)
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("X-Real-IP", e.Address())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
