package protocolserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
)

const (
	echoCommand     uint16 = 100
	callbackCommand uint16 = 101
	blockedCommand  uint16 = 102
)

type shortWriteConn struct{ net.Conn }

func (c shortWriteConn) Write(data []byte) (int, error) {
	return c.Conn.Write(data[:min(len(data), 3)])
}

func registerHandler(t *testing.T, srv *Server, id uint16, handler protocolcommandstore.Handler) {
	t.Helper()
	if err := srv.cs.RegisterHandler(id, handler); err != nil {
		t.Fatal(err)
	}
}

func echoHandler(_ *protocolcommandstore.ConnCtx, _ *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
	return &protocol.Response{Code: protocol.StatusCodeOK, Payload: bytes.Clone(cmd.Payload)}, nil
}

// Use short writes on both ends to exercise frame serialization on a full
// duplex stream rather than relying on a TCP write to send a whole frame.
func connectPeers(t *testing.T, srv, agent *Server) (*connection, *connection) {
	t.Helper()
	serverConn, agentConn := net.Pipe()
	serverPeer := newConnection(shortWriteConn{serverConn})
	agentPeer := newConnection(shortWriteConn{agentConn})
	srv.connectionsMu.Lock()
	srv.connections[serverPeer.ID] = serverPeer
	srv.connectionsMu.Unlock()
	agent.connectionsMu.Lock()
	agent.connections[agentPeer.ID] = agentPeer
	agent.clientConn = agentPeer
	agent.connectionsMu.Unlock()

	serverDone := make(chan struct{})
	agentDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		srv.serveConnection(serverPeer)
	}()
	go func() {
		defer close(agentDone)
		agent.serveConnection(agentPeer)
	}()
	t.Cleanup(func() {
		serverPeer.close()
		agentPeer.close()
		await(t, serverDone)
		await(t, agentDone)
	})
	return serverPeer, agentPeer
}

func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for peer")
		var zero T
		return zero
	}
}

func awaitConnection(t *testing.T, srv *Server) *protocolcommandstore.ConnCtx {
	t.Helper()
	return awaitReplacementConnection(t, srv, "")
}

func awaitReplacementConnection(t *testing.T, srv *Server, previousID string) *protocolcommandstore.ConnCtx {
	t.Helper()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		for _, conn := range srv.GetConnections() {
			if conn.ID != previousID {
				return conn
			}
		}
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatal("connection was not registered")
		}
	}
}

func assertResponse(t *testing.T, resp *protocol.Response, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Code != protocol.StatusCodeOK || string(resp.Payload) != want {
		t.Fatalf("response = %+v, want OK with %q", resp, want)
	}
}

func assertNoPendingCommands(t *testing.T, conn *connection) {
	t.Helper()
	conn.pendingCmdsMu.Lock()
	defer conn.pendingCmdsMu.Unlock()
	if len(conn.pendingCmds) != 0 {
		t.Fatalf("connection has %d pending commands", len(conn.pendingCmds))
	}
}

func TestConnectToBidirectional(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	agent := New("", protocolcommandstore.New())
	t.Cleanup(agent.Disconnect)
	registerHandler(t, srv, echoCommand, echoHandler)
	registerHandler(t, agent, echoCommand, echoHandler)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := agent.ConnectTo(listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	agentCtx := awaitConnection(t, agent)
	defer agentCtx.Conn.Close()
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
	serverCtx := awaitConnection(t, srv)

	resp, err := agent.SendCmd(&protocol.Command{ID: echoCommand, Payload: []byte("agent to server")})
	assertResponse(t, resp, err, "agent to server")
	resp, err = srv.SendCmdTo(serverCtx.ID, &protocol.Command{ID: echoCommand, Payload: []byte("server to agent")})
	assertResponse(t, resp, err, "server to agent")
	resp, err = agent.SendCmdTo(agentCtx.ID, &protocol.Command{ID: echoCommand, Payload: []byte("outbound by ID")})
	assertResponse(t, resp, err, "outbound by ID")

	// Callers must not be able to mutate the live connection map.
	snapshot := srv.GetConnections()
	delete(snapshot, serverCtx.ID)
	if srv.GetConnection(serverCtx.ID) != serverCtx {
		t.Fatal("GetConnections exposed the live connection map")
	}
	if len(srv.GetConnections()) != 1 || len(agent.GetConnections()) != 1 {
		t.Fatal("expected one connection on each peer")
	}
}

func TestConcurrentBidirectionalCommands(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	agent := New("", protocolcommandstore.New())
	registerHandler(t, srv, echoCommand, echoHandler)
	registerHandler(t, agent, echoCommand, echoHandler)
	serverPeer, agentPeer := connectPeers(t, srv, agent)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type result struct {
		resp *protocol.Response
		want string
		err  error
	}
	const count = 20
	results := make(chan result, 2*count)
	start := make(chan struct{})
	for i := range count {
		for _, fromServer := range []bool{false, true} {
			go func() {
				<-start
				payload := fmt.Sprintf("server=%v command=%02d %s", fromServer, i, bytes.Repeat([]byte("x"), 64))
				cmd := &protocol.Command{ID: echoCommand, Payload: []byte(payload)}
				var resp *protocol.Response
				var err error
				if fromServer {
					resp, err = srv.SendCmdToContext(ctx, serverPeer.ID, cmd)
				} else {
					resp, err = agent.SendCmdContext(ctx, cmd)
				}
				if err == nil && resp.ReqID != cmd.ReqID {
					err = fmt.Errorf("response ID = %d, want %d", resp.ReqID, cmd.ReqID)
				}
				results <- result{resp: resp, want: payload, err: err}
			}()
		}
	}
	close(start)
	var received []result
	for range 2 * count {
		r := await(t, results)
		assertResponse(t, r.resp, r.err, r.want)
		received = append(received, r)
	}
	// Returned response payloads must survive reuse of the receive buffers.
	for _, r := range received {
		assertResponse(t, r.resp, r.err, r.want)
	}
	assertNoPendingCommands(t, serverPeer)
	assertNoPendingCommands(t, agentPeer)
}

func TestResponsesStayOnTheirConnection(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	started := make(chan uint32, 2)
	release := make(chan struct{})
	defer close(release)
	var peers []*connection
	for _, name := range []string{"agent A", "agent B"} {
		agent := New("", protocolcommandstore.New())
		registerHandler(t, agent, echoCommand, func(ctx *protocolcommandstore.ConnCtx, _ *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
			started <- cmd.ReqID
			select {
			case <-release:
			case <-ctx.Ctx.Done():
			}
			return &protocol.Response{Code: protocol.StatusCodeOK, Payload: []byte(name)}, nil
		})
		peer, _ := connectPeers(t, srv, agent)
		peers = append(peers, peer)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type result struct {
		resp *protocol.Response
		err  error
		want string
	}
	results := make(chan result, 2)
	for i, peer := range peers {
		go func() {
			resp, err := srv.SendCmdToContext(ctx, peer.ID, &protocol.Command{ID: echoCommand})
			results <- result{resp: resp, err: err, want: []string{"agent A", "agent B"}[i]}
		}()
	}
	// Both connections deliberately have the same in-flight request ID.
	if first, second := await(t, started), await(t, started); first != second {
		t.Fatalf("request IDs = %d and %d, want equal IDs on different connections", first, second)
	}
	release <- struct{}{}
	release <- struct{}{}
	for range 2 {
		r := await(t, results)
		assertResponse(t, r.resp, r.err, r.want)
	}
}

func TestHandlerCanSendReverseCommand(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	agent := New("", protocolcommandstore.New())
	registerHandler(t, srv, echoCommand, echoHandler)
	registerHandler(t, agent, echoCommand, echoHandler)
	registerHandler(t, srv, callbackCommand, func(ctx *protocolcommandstore.ConnCtx, _ *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
		return srv.SendCmdToContext(ctx.Ctx, ctx.ID, &protocol.Command{ID: echoCommand, Payload: cmd.Payload})
	})
	registerHandler(t, agent, callbackCommand, func(ctx *protocolcommandstore.ConnCtx, _ *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
		return agent.SendCmdContext(ctx.Ctx, &protocol.Command{ID: echoCommand, Payload: cmd.Payload})
	})
	serverPeer, _ := connectPeers(t, srv, agent)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := agent.SendCmdContext(ctx, &protocol.Command{ID: callbackCommand, Payload: []byte("callback to agent")})
	assertResponse(t, resp, err, "callback to agent")
	resp, err = srv.SendCmdToContext(ctx, serverPeer.ID, &protocol.Command{ID: callbackCommand, Payload: []byte("callback to server")})
	assertResponse(t, resp, err, "callback to server")
}

func TestCanceledCommandDoesNotBlockConnection(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelRequest), func(t *testing.T) {
			srv := New("", protocolcommandstore.New())
			agent := New("", protocolcommandstore.New())
			registerHandler(t, agent, echoCommand, echoHandler)
			started := make(chan struct{})
			release := make(chan struct{})
			defer close(release)
			registerHandler(t, agent, blockedCommand, func(ctx *protocolcommandstore.ConnCtx, _ *protocol.Message, cmd *protocol.Command) (*protocol.Response, error) {
				close(started)
				select {
				case <-release:
				case <-ctx.Ctx.Done():
				}
				return &protocol.Response{Code: protocol.StatusCodeOK, Payload: bytes.Clone(cmd.Payload)}, nil
			})
			peer, _ := connectPeers(t, srv, agent)
			var ctx context.Context
			var cancel context.CancelFunc
			if cancelRequest {
				ctx, cancel = context.WithCancel(context.Background())
			} else {
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := srv.SendCmdToContext(ctx, peer.ID, &protocol.Command{ID: blockedCommand, Payload: []byte("late response")})
				result <- err
			}()
			await(t, started)
			wantErr := context.DeadlineExceeded
			if cancelRequest {
				cancel()
				wantErr = context.Canceled
			}
			if err := await(t, result); !errors.Is(err, wantErr) {
				t.Fatalf("SendCmdToContext error = %v, want %v", err, wantErr)
			}
			assertNoPendingCommands(t, peer)
			resp, err := srv.SendCmdTo(peer.ID, &protocol.Command{ID: echoCommand, Payload: []byte("while handler waits")})
			assertResponse(t, resp, err, "while handler waits")
			release <- struct{}{}
			resp, err = srv.SendCmdTo(peer.ID, &protocol.Command{ID: echoCommand, Payload: []byte("after late response")})
			assertResponse(t, resp, err, "after late response")
		})
	}
}

func TestDisconnectFailsPendingCommands(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	agent := New("", protocolcommandstore.New())
	started := make(chan struct{})
	registerHandler(t, agent, blockedCommand, func(ctx *protocolcommandstore.ConnCtx, _ *protocol.Message, _ *protocol.Command) (*protocol.Response, error) {
		close(started)
		<-ctx.Ctx.Done()
		return &protocol.Response{Code: protocol.StatusCodeOK}, nil
	})
	serverPeer, agentPeer := connectPeers(t, srv, agent)
	result := make(chan error, 1)
	go func() {
		_, err := srv.SendCmdTo(serverPeer.ID, &protocol.Command{ID: blockedCommand})
		result <- err
	}()
	await(t, started)
	agentPeer.close()
	if err := await(t, result); !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("SendCmdTo error = %v, want %v", err, ErrConnectionClosed)
	}
	await(t, serverPeer.Ctx.Done())
	assertNoPendingCommands(t, serverPeer)
}

func TestWriteFailureCleansPendingCommands(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	local, remote := net.Pipe()
	peer := newConnection(local)
	defer peer.close()
	remote.Close()
	srv.connections[peer.ID] = peer
	_, err := srv.SendCmdTo(peer.ID, &protocol.Command{ID: echoCommand})
	if err == nil {
		t.Fatal("SendCmdTo succeeded on a closed connection")
	}
	assertNoPendingCommands(t, peer)
	await(t, peer.Ctx.Done())
}

func TestBlockedWriteCanBeCanceled(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	local, remote := net.Pipe()
	defer remote.Close()
	peer := newConnection(local)
	defer peer.close()
	srv.connections[peer.ID] = peer
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := srv.SendCmdToContext(ctx, peer.ID, &protocol.Command{ID: echoCommand})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendCmdToContext error = %v, want %v", err, context.DeadlineExceeded)
	}
	assertNoPendingCommands(t, peer)
	await(t, peer.Ctx.Done())
}

func TestSendCmdWithoutConnection(t *testing.T) {
	srv := New("", protocolcommandstore.New())
	if _, err := srv.SendCmd(&protocol.Command{}); !errors.Is(err, ErrClientNotConnected) {
		t.Fatalf("SendCmd error = %v, want %v", err, ErrClientNotConnected)
	}
	if _, err := srv.SendCmdTo("missing", &protocol.Command{}); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("SendCmdTo error = %v, want %v", err, ErrConnectionNotFound)
	}
}
