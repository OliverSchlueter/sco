package protocolserver

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
)

func listenTCP(t *testing.T, addr string) *net.TCPListener {
	t.Helper()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener.(*net.TCPListener)
}

func acceptTCPPeer(t *testing.T, listener *net.TCPListener, srv *Server, previousID string) *protocolcommandstore.ConnCtx {
	t.Helper()
	if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConnection(conn)
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		await(t, done)
	})
	return awaitReplacementConnection(t, srv, previousID)
}

func assertNoReconnect(t *testing.T, listener *net.TCPListener) {
	t.Helper()
	if err := listener.SetDeadline(time.Now().Add(reconnectDelay + 500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.Accept()
	if err == nil {
		_ = conn.Close()
		t.Fatal("unexpected automatic reconnection")
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("Accept error = %v, want deadline exceeded", err)
	}
}

func TestConnectToReconnects(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	agent := New("", protocolcommandstore.New())
	registerHandler(t, srv, echoCommand, echoHandler)
	registerHandler(t, agent, echoCommand, echoHandler)
	started := make(chan struct{})
	registerHandler(t, srv, blockedCommand, func(ctx *protocolcommandstore.ConnCtx, _ *protocol.Message, _ *protocol.Command) (*protocol.Response, error) {
		close(started)
		<-ctx.Ctx.Done()
		return &protocol.Response{Code: protocol.StatusCodeOK}, nil
	})
	listener := listenTCP(t, "127.0.0.1:0")
	t.Cleanup(agent.Disconnect)
	if err := agent.ConnectTo(listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	serverCtx := acceptTCPPeer(t, listener, srv, "")
	agentCtx := awaitConnection(t, agent)
	pending := make(chan error, 1)
	go func() {
		_, err := agent.SendCmd(&protocol.Command{ID: blockedCommand})
		pending <- err
	}()
	await(t, started)

	for i := range 2 {
		previousServer, previousAgent := serverCtx, agentCtx
		_ = serverCtx.Conn.Close()
		await(t, previousServer.Ctx.Done())
		await(t, previousAgent.Ctx.Done())
		if i == 0 {
			if err := await(t, pending); !errors.Is(err, ErrConnectionClosed) {
				t.Fatalf("pending command error = %v, want %v", err, ErrConnectionClosed)
			}
		}
		serverCtx = acceptTCPPeer(t, listener, srv, previousServer.ID)
		agentCtx = awaitReplacementConnection(t, agent, previousAgent.ID)
		if agent.GetConnection(previousAgent.ID) != nil || srv.GetConnection(previousServer.ID) != nil {
			t.Fatal("disconnected peer was not removed")
		}
		if len(agent.GetConnections()) != 1 || len(srv.GetConnections()) != 1 {
			t.Fatal("expected one connection on each peer after reconnect")
		}
		resp, err := agent.SendCmd(&protocol.Command{ID: echoCommand, Payload: []byte("reconnected agent")})
		assertResponse(t, resp, err, "reconnected agent")
		resp, err = srv.SendCmdTo(serverCtx.ID, &protocol.Command{ID: echoCommand, Payload: []byte("reconnected server")})
		assertResponse(t, resp, err, "reconnected server")
		resp, err = agent.SendCmdTo(agentCtx.ID, &protocol.Command{ID: echoCommand, Payload: []byte("reconnected by ID")})
		assertResponse(t, resp, err, "reconnected by ID")
	}
}

func TestConnectToReconnectsAfterServerRestart(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	agent := New("", protocolcommandstore.New())
	registerHandler(t, srv, echoCommand, echoHandler)
	listener := listenTCP(t, "127.0.0.1:0")
	t.Cleanup(agent.Disconnect)
	addr := listener.Addr().String()
	if err := agent.ConnectTo(addr); err != nil {
		t.Fatal(err)
	}
	serverCtx := acceptTCPPeer(t, listener, srv, "")
	agentCtx := awaitConnection(t, agent)
	_ = listener.Close()
	_ = serverCtx.Conn.Close()
	await(t, serverCtx.Ctx.Done())
	await(t, agentCtx.Ctx.Done())

	// Keep the port closed long enough for a reconnect attempt to fail.
	<-time.After(2 * reconnectDelay)
	if _, err := agent.SendCmd(&protocol.Command{ID: echoCommand}); !errors.Is(err, ErrClientNotConnected) {
		t.Fatalf("SendCmd during outage error = %v, want %v", err, ErrClientNotConnected)
	}
	listener = listenTCP(t, addr)
	acceptTCPPeer(t, listener, srv, serverCtx.ID)
	awaitReplacementConnection(t, agent, agentCtx.ID)
	resp, err := agent.SendCmd(&protocol.Command{ID: echoCommand, Payload: []byte("server restarted")})
	assertResponse(t, resp, err, "server restarted")
}

func TestConnectToReplacesReconnectTarget(t *testing.T) {
	for _, disconnected := range []bool{false, true} {
		t.Run(fmt.Sprintf("disconnected=%v", disconnected), func(t *testing.T) {
			srv := New("", protocolcommandstore.New())
			agent := New("", protocolcommandstore.New())
			registerHandler(t, srv, echoCommand, echoHandler)
			oldListener := listenTCP(t, "127.0.0.1:0")
			newListener := listenTCP(t, "127.0.0.1:0")
			t.Cleanup(agent.Disconnect)
			if err := agent.ConnectTo(oldListener.Addr().String()); err != nil {
				t.Fatal(err)
			}
			oldServer := acceptTCPPeer(t, oldListener, srv, "")
			oldAgent := awaitConnection(t, agent)
			if disconnected {
				_ = oldServer.Conn.Close()
				await(t, oldAgent.Ctx.Done())
			}
			if err := agent.ConnectTo(newListener.Addr().String()); err != nil {
				t.Fatal(err)
			}
			await(t, oldServer.Ctx.Done())
			await(t, oldAgent.Ctx.Done())
			acceptTCPPeer(t, newListener, srv, oldServer.ID)
			awaitReplacementConnection(t, agent, oldAgent.ID)
			assertNoReconnect(t, oldListener)
			resp, err := agent.SendCmd(&protocol.Command{ID: echoCommand, Payload: []byte("new endpoint")})
			assertResponse(t, resp, err, "new endpoint")
		})
	}
}

func TestDisconnectStopsReconnect(t *testing.T) {
	for _, disconnected := range []bool{false, true} {
		t.Run(fmt.Sprintf("disconnected=%v", disconnected), func(t *testing.T) {
			srv := New("", protocolcommandstore.New())
			agent := New("", protocolcommandstore.New())
			registerHandler(t, srv, echoCommand, echoHandler)
			listener := listenTCP(t, "127.0.0.1:0")
			t.Cleanup(agent.Disconnect)
			if err := agent.ConnectTo(listener.Addr().String()); err != nil {
				t.Fatal(err)
			}
			serverCtx := acceptTCPPeer(t, listener, srv, "")
			agentCtx := awaitConnection(t, agent)
			if disconnected {
				_ = serverCtx.Conn.Close()
				await(t, agentCtx.Ctx.Done())
			}
			agent.Disconnect()
			agent.Disconnect()
			await(t, serverCtx.Ctx.Done())
			await(t, agentCtx.Ctx.Done())
			if len(agent.GetConnections()) != 0 {
				t.Fatal("Disconnect did not remove the outbound connection")
			}
			if _, err := agent.SendCmd(&protocol.Command{}); !errors.Is(err, ErrClientNotConnected) {
				t.Fatalf("SendCmd after Disconnect error = %v, want %v", err, ErrClientNotConnected)
			}
			assertNoReconnect(t, listener)
			if err := agent.ConnectTo(listener.Addr().String()); err != nil {
				t.Fatal(err)
			}
			acceptTCPPeer(t, listener, srv, serverCtx.ID)
			resp, err := agent.SendCmd(&protocol.Command{ID: echoCommand, Payload: []byte("connected again")})
			assertResponse(t, resp, err, "connected again")
		})
	}
}

func TestConnectToFailurePreservesConnection(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	agent := New("", protocolcommandstore.New())
	registerHandler(t, srv, echoCommand, echoHandler)
	listener := listenTCP(t, "127.0.0.1:0")
	t.Cleanup(agent.Disconnect)
	if err := agent.ConnectTo("127.0.0.1:invalid"); err == nil {
		t.Fatal("ConnectTo succeeded for an invalid address")
	}
	if len(agent.GetConnections()) != 0 {
		t.Fatal("failed ConnectTo registered a connection")
	}
	if err := agent.ConnectTo(listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	serverCtx := acceptTCPPeer(t, listener, srv, "")
	agentCtx := awaitConnection(t, agent)
	if err := agent.ConnectTo("127.0.0.1:invalid"); err == nil {
		t.Fatal("ConnectTo succeeded for an invalid address")
	}
	resp, err := agent.SendCmd(&protocol.Command{ID: echoCommand, Payload: []byte("original endpoint")})
	assertResponse(t, resp, err, "original endpoint")
	_ = serverCtx.Conn.Close()
	await(t, serverCtx.Ctx.Done())
	await(t, agentCtx.Ctx.Done())
	acceptTCPPeer(t, listener, srv, serverCtx.ID)
	awaitReplacementConnection(t, agent, agentCtx.ID)
	resp, err = agent.SendCmd(&protocol.Command{ID: echoCommand, Payload: []byte("original reconnect target")})
	assertResponse(t, resp, err, "original reconnect target")
}
