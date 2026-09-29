package protocolserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
)

const commandTimeout = 30 * time.Second

type Server struct {
	port string
	cs   *protocolcommandstore.Store

	connectionsMu sync.RWMutex
	connections   map[string]*connection
	clientConn    *connection
}

func New(port string, commandStore *protocolcommandstore.Store) *Server {
	return &Server{
		port:        port,
		cs:          commandStore,
		connections: make(map[string]*connection),
	}
}

// GetConnections returns a snapshot of the accepted and outbound connections.
func (s *Server) GetConnections() map[string]*protocolcommandstore.ConnCtx {
	s.connectionsMu.RLock()
	defer s.connectionsMu.RUnlock()

	connections := make(map[string]*protocolcommandstore.ConnCtx, len(s.connections))
	for id, conn := range s.connections {
		connections[id] = conn.ConnCtx
	}
	return connections
}

func (s *Server) GetConnection(id string) *protocolcommandstore.ConnCtx {
	s.connectionsMu.RLock()
	defer s.connectionsMu.RUnlock()

	if conn := s.connections[id]; conn != nil {
		return conn.ConnCtx
	}
	return nil
}

// Start starts the server and listens for incoming connections.
// It blocks until the server is stopped.
func (s *Server) Start() {
	ln, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		slog.Error("Failed to start protocol server", sloki.WrapError(err))
		return
	}

	slog.Debug("Protocol server started", slog.String("port", s.port))

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Warn("Failed to accept connection", sloki.WrapError(err))
			continue
		}

		go s.handleConnection(conn)
	}
}

// ConnectTo establishes a connection to a remote server.
func (s *Server) ConnectTo(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	peer := newConnection(conn)
	s.connectionsMu.Lock()
	previous := s.clientConn
	s.clientConn = peer
	s.connections[peer.ID] = peer
	s.connectionsMu.Unlock()
	if previous != nil {
		previous.close()
	}

	go s.serveConnection(peer)
	return nil
}

// handleConnection manages the lifecycle of a single client connection.
// It reads messages in a loop until the connection is closed.
func (s *Server) handleConnection(conn net.Conn) {
	peer := newConnection(conn)
	s.connectionsMu.Lock()
	s.connections[peer.ID] = peer
	s.connectionsMu.Unlock()

	s.serveConnection(peer)
}

// serveConnection uses the same receive loop for accepted and outbound peers.
func (s *Server) serveConnection(peer *connection) {
	defer func() {
		peer.close()
		s.connectionsMu.Lock()
		delete(s.connections, peer.ID)
		if s.clientConn == peer {
			s.clientConn = nil
		}
		s.connectionsMu.Unlock()
	}()

	slog.Debug("New connection established", slog.String("conn_id", peer.ID))

	for {
		if s.handleMessage(peer) {
			break
		}
	}
}

// handleMessage reads and processes a single message from the connection.
// It returns true if the connection should be closed.
func (s *Server) handleMessage(ctx *connection) bool {
	conn := ctx.Conn

	frameBuf := protocol.GetRequestBufferFromPool()
	defer func() { protocol.PutRequestBufferToPool(frameBuf) }()

	frame, err := protocol.V1.ReadFrameInto(conn, frameBuf)
	if err != nil {
		if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
			slog.Info("Connection closed by client", slog.String("conn_id", ctx.ID))
			return true
		}

		if errors.Is(err, protocol.ErrFrameLengthInvalid) {
			s.writeResponse(ctx, &protocol.Response{
				Code:    protocol.StatusInvalidMessage,
				Payload: []byte(err.Error()),
			})
		} else {
			slog.Error("Failed to read frame", slog.String("conn_id", ctx.ID), sloki.WrapError(err))
		}

		return true
	}
	frameBuf = frame

	msg := protocol.GetMessageFromPool()
	defer protocol.PutMessageToPool(msg)
	if err := protocol.V1.DecodeMessageInto(frame, msg); err != nil {
		return s.writeResponse(ctx, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte(err.Error()),
		})
	}

	if msg.ProtocolVersion != byte(protocol.Version1) {
		return s.writeResponse(ctx, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte("Only protocol version 1 is supported"),
		})
	}

	switch msg.Type {
	case byte(protocol.MessageTypeCommand):
		// Handlers may send a command back to this peer and wait for its reply.
		// Keep reading responses, and give the handler its own payload buffer.
		commandMsg := *msg
		commandMsg.Payload = bytes.Clone(msg.Payload)
		go s.handleCommand(ctx, &commandMsg)
		return false
	case byte(protocol.MessageTypeResponse):
		return s.handleResponse(ctx, msg)
	default:
		return s.writeResponse(ctx, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte("Only command and response messages are allowed"),
		})
	}
}

func (s *Server) handleCommand(ctx *connection, msg *protocol.Message) {
	cmd := protocol.GetCommandFromPool()
	defer protocol.PutCommandToPool(cmd)

	if err := protocol.V1.DecodeCommandInto(msg, cmd); err != nil {
		s.writeResponse(ctx, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte(err.Error()),
		})
		return
	}

	resp := s.cs.Execute(ctx.ConnCtx, msg, cmd)

	// Ensure the response has the same ReqID as the command for proper correlation on the client side
	resp.ReqID = cmd.ReqID

	s.writeResponse(ctx, resp)

	slog.Debug(
		"Processed command",
		slog.String("conn_id", ctx.ID),
		slog.Int("command_id", int(cmd.ID)),
		slog.String("payload", string(cmd.Payload)),
	)
}

func (s *Server) handleResponse(ctx *connection, msg *protocol.Message) bool {
	resp, err := protocol.V1.DecodeResponse(msg)
	if err != nil {
		return s.writeResponse(ctx, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte(err.Error()),
		})
	}

	ctx.pendingCmdsMu.Lock()
	respChan, exists := ctx.pendingCmds[resp.ReqID]
	if !exists {
		ctx.pendingCmdsMu.Unlock()
		slog.Warn(
			"Received response with unknown ID",
			slog.String("conn_id", ctx.ID),
			slog.String("response_id", strconv.Itoa(int(resp.ReqID))),
		)
		return false
	}

	delete(ctx.pendingCmds, resp.ReqID)
	ctx.pendingCmdsMu.Unlock()
	// The receive buffer returns to the pool after this method completes.
	resp.Payload = bytes.Clone(resp.Payload)
	respChan <- resp

	return false
}

// writeResponse returns true if the connection should be closed.
func (s *Server) writeResponse(conn *connection, resp *protocol.Response) bool {
	msg := protocol.GetMessageFromPool()
	defer protocol.PutMessageToPool(msg)

	payloadBuf := protocol.GetResponseBufferFromPool()
	defer func() { protocol.PutResponseBufferToPool(payloadBuf) }()

	msg.ProtocolVersion = byte(protocol.Version1)
	msg.Flags = 0x00
	msg.Type = byte(protocol.MessageTypeResponse)
	payloadBuf = protocol.V1.EncodeResponseInto(resp, payloadBuf)
	msg.Payload = payloadBuf

	msgDataBuf := protocol.GetResponseBufferFromPool()
	defer func() { protocol.PutResponseBufferToPool(msgDataBuf) }()

	msgDataBuf = protocol.V1.EncodeMessageInto(msg, msgDataBuf)
	ctx, cancel := context.WithTimeout(conn.Ctx, commandTimeout)
	defer cancel()
	if err := conn.writeFrame(ctx, msgDataBuf); err != nil {
		slog.Warn("Failed to write response", sloki.WrapError(err))
		return true
	}
	return false
}

// SendCmd sends a command over the connection established by ConnectTo.
func (s *Server) SendCmd(cmd *protocol.Command) (*protocol.Response, error) {
	return s.SendCmdContext(context.Background(), cmd)
}

// SendCmdContext is SendCmd with cancellation or an earlier deadline.
func (s *Server) SendCmdContext(ctx context.Context, cmd *protocol.Command) (*protocol.Response, error) {
	s.connectionsMu.RLock()
	conn := s.clientConn
	s.connectionsMu.RUnlock()
	if conn == nil {
		return nil, ErrClientNotConnected
	}
	return s.sendCmd(ctx, conn, cmd)
}

// SendCmdTo sends a command to an accepted or outbound connection by its ID.
// Both peers can send commands over the same connection at the same time.
func (s *Server) SendCmdTo(connectionID string, cmd *protocol.Command) (*protocol.Response, error) {
	return s.SendCmdToContext(context.Background(), connectionID, cmd)
}

// SendCmdToContext is SendCmdTo with cancellation or an earlier deadline.
func (s *Server) SendCmdToContext(ctx context.Context, connectionID string, cmd *protocol.Command) (*protocol.Response, error) {
	s.connectionsMu.RLock()
	conn := s.connections[connectionID]
	s.connectionsMu.RUnlock()
	if conn == nil {
		return nil, ErrConnectionNotFound
	}
	return s.sendCmd(ctx, conn, cmd)
}

func (s *Server) sendCmd(ctx context.Context, conn *connection, cmd *protocol.Command) (*protocol.Response, error) {
	requestCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	if err := requestCtx.Err(); err != nil {
		return nil, err
	}
	if conn.Ctx.Err() != nil {
		return nil, ErrConnectionClosed
	}

	startTime := time.Now()

	respChan := make(chan *protocol.Response, 1)
	conn.pendingCmdsMu.Lock()
	cmd.ReqID = conn.requestIDCounter.Add(1)
	for conn.pendingCmds[cmd.ReqID] != nil {
		cmd.ReqID = conn.requestIDCounter.Add(1)
	}
	reqID := cmd.ReqID
	conn.pendingCmds[reqID] = respChan
	conn.pendingCmdsMu.Unlock()
	defer func() {
		conn.pendingCmdsMu.Lock()
		delete(conn.pendingCmds, reqID)
		conn.pendingCmdsMu.Unlock()
	}()

	cmdMsg := protocol.Message{
		ProtocolVersion: byte(protocol.Version1),
		Flags:           0x00,
		Type:            byte(protocol.MessageTypeCommand),
		Payload:         protocol.V1.EncodeCommand(cmd),
	}

	cmdData := protocol.V1.EncodeMessage(&cmdMsg)
	if err := conn.writeFrame(requestCtx, cmdData); err != nil {
		return nil, commandError(ctx, err)
	}

	slog.Debug(
		"Sent command to peer",
		slog.String("conn_id", conn.ID),
		slog.String("id", strconv.Itoa(int(cmd.ID))),
		slog.String("payload_size", strconv.Itoa(len(cmd.Payload))),
	)

	// Wait for the response or a timeout
	select {
	case resp := <-respChan:
		duration := time.Since(startTime)
		slog.Debug(
			"Received response from peer",
			slog.String("conn_id", conn.ID),
			slog.String("status_code", strconv.Itoa(int(resp.Code))),
			slog.String("payload_size", strconv.Itoa(len(resp.Payload))),
			slog.Duration("duration", duration),
		)
		return resp, nil
	case <-requestCtx.Done():
		return nil, commandError(ctx, requestCtx.Err())
	case <-conn.Ctx.Done():
		return nil, ErrConnectionClosed
	}
}

func commandError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var networkErr net.Error
	isNetworkTimeout := errors.As(err, &networkErr) && networkErr.Timeout()
	if errors.Is(err, context.DeadlineExceeded) || isNetworkTimeout {
		if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
			return context.DeadlineExceeded
		}
		return ErrCommandTimeout
	}
	return err
}
