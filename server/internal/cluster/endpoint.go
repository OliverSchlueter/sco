package cluster

import (
	"io"
	"net"
	"net/http"
)

type Endpoint struct {
	NodeID string
	Host   string
	Port   string
}

func (e *Endpoint) Address() string {
	return e.Host + ":" + e.Port
}

func (e *Endpoint) ForwardTcpConn(conn net.Conn) error {
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

func (e *Endpoint) ForwardHttpReq(req *http.Request) (*http.Response, error) {
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
