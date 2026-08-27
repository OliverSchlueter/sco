package protocolserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OliverSchlueter/goutils/idgen"
	"github.com/OliverSchlueter/goutils/sloki"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
)

type Server struct {
	port string
	cs   *protocolcommandstore.Store

	connectionsMu sync.RWMutex
	connections   map[string]*protocolcommandstore.ConnCtx

	clientConn       net.Conn
	requestIDCounter atomic.Uint32
	pendingCmds      map[uint32]chan *protocol.Response
	pendingCmdsMu    sync.Mutex
}

func New(port string, commandStore *protocolcommandstore.Store) *Server {
	return &Server{
		port:        port,
		cs:          commandStore,
		connections: make(map[string]*protocolcommandstore.ConnCtx),
		pendingCmds: make(map[uint32]chan *protocol.Response),
	}
}

func (s *Server) GetConnections() map[string]*protocolcommandstore.ConnCtx {
	s.connectionsMu.RLock()
	defer s.connectionsMu.RUnlock()

	return s.connections
}

func (s *Server) GetConnection(id string) *protocolcommandstore.ConnCtx {
	s.connectionsMu.RLock()
	defer s.connectionsMu.RUnlock()

	return s.connections[id]
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

	s.clientConn = conn

	go s.handleConnection(conn)
	return nil
}

// handleConnection manages the lifecycle of a single client connection.
// It reads messages in a loop until the connection is closed.
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	ctx := &protocolcommandstore.ConnCtx{
		ID:   idgen.GenerateID(16),
		Conn: conn,
		Ctx:  context.Background(),
	}
	s.connectionsMu.Lock()
	s.connections[ctx.ID] = ctx
	s.connectionsMu.Unlock()

	defer func() {
		s.connectionsMu.Lock()
		delete(s.connections, ctx.ID)
		s.connectionsMu.Unlock()
	}()

	slog.Debug("New connection established", slog.String("conn_id", ctx.ID))

	for {
		if s.handleMessage(ctx) {
			break
		}
	}
}

// handleMessage reads and processes a single message from the connection.
// It returns true if the connection should be closed.
func (s *Server) handleMessage(ctx *protocolcommandstore.ConnCtx) bool {
	conn := ctx.Conn

	frameBuf := protocol.GetRequestBufferFromPool()
	defer protocol.PutRequestBufferToPool(frameBuf)

	frame, err := protocol.V1.ReadFrameInto(conn, frameBuf)
	if err != nil {
		if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
			slog.Info("Connection closed by client", slog.String("conn_id", ctx.ID))
			return true
		}

		if errors.Is(err, protocol.ErrFrameLengthInvalid) {
			s.writeResponse(conn, &protocol.Response{
				Code:    protocol.StatusInvalidMessage,
				Payload: []byte(err.Error()),
			})
		} else {
			slog.Error("Failed to read frame", slog.String("conn_id", ctx.ID), sloki.WrapError(err))
		}

		return false
	}

	msg := protocol.GetMessageFromPool()
	defer protocol.PutMessageToPool(msg)
	if err := protocol.V1.DecodeMessageInto(frame, msg); err != nil {
		s.writeResponse(conn, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte(err.Error()),
		})

		return false
	}

	if msg.ProtocolVersion != byte(protocol.Version1) {
		s.writeResponse(conn, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte("Only protocol version 1 is supported"),
		})

		return false
	}

	switch msg.Type {
	case byte(protocol.MessageTypeCommand):
		return s.handleCommand(ctx, msg)
	case byte(protocol.MessageTypeResponse):
		return s.handleResponse(ctx, msg)
	default:
		s.writeResponse(conn, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte("Only command and response messages are allowed"),
		})
		return false
	}
}

func (s *Server) handleCommand(ctx *protocolcommandstore.ConnCtx, msg *protocol.Message) bool {
	cmd := protocol.GetCommandFromPool()
	defer protocol.PutCommandToPool(cmd)

	if err := protocol.V1.DecodeCommandInto(msg, cmd); err != nil {
		s.writeResponse(ctx.Conn, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte(err.Error()),
		})
		return false
	}

	resp := s.cs.Execute(ctx, msg, cmd)

	// Ensure the response has the same ReqID as the command for proper correlation on the client side
	resp.ReqID = cmd.ReqID

	s.writeResponse(ctx.Conn, resp)

	slog.Debug(
		"Processed command",
		slog.String("conn_id", ctx.ID),
		slog.Int("command_id", int(cmd.ID)),
		slog.String("payload", string(cmd.Payload)),
	)
	return false
}

func (s *Server) handleResponse(ctx *protocolcommandstore.ConnCtx, msg *protocol.Message) bool {
	resp, err := protocol.V1.DecodeResponse(msg)
	if err != nil {
		s.writeResponse(ctx.Conn, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte(err.Error()),
		})
		return false
	}

	s.pendingCmdsMu.Lock()
	respChan, exists := s.pendingCmds[resp.ReqID]
	if !exists {
		s.pendingCmdsMu.Unlock()
		slog.Warn(
			"Received response with unknown ID",
			slog.String("conn_id", ctx.ID),
			slog.String("response_id", strconv.Itoa(int(resp.ReqID))),
		)
		return false
	}

	delete(s.pendingCmds, resp.ReqID)
	s.pendingCmdsMu.Unlock()
	respChan <- resp

	return false
}

func (s *Server) writeResponse(conn net.Conn, resp *protocol.Response) {
	msg := protocol.GetMessageFromPool()
	defer protocol.PutMessageToPool(msg)

	payloadBuf := protocol.GetResponseBufferFromPool()
	defer protocol.PutResponseBufferToPool(payloadBuf)

	msg.ProtocolVersion = byte(protocol.Version1)
	msg.Flags = 0x00
	msg.Type = byte(protocol.MessageTypeResponse)
	msg.Payload = protocol.V1.EncodeResponseInto(resp, payloadBuf)

	msgDataBuf := protocol.GetResponseBufferFromPool()
	defer protocol.PutResponseBufferToPool(msgDataBuf)

	msgDataBuf = protocol.V1.EncodeMessageInto(msg, msgDataBuf)
	if err := protocol.V1.WriteFrame(conn, msgDataBuf); err != nil {
		slog.Warn("Failed to write response", sloki.WrapError(err))
	}
}

// SendCmd sends a command to a client and waits for the response.
func (s *Server) SendCmd(cmd *protocol.Command) (*protocol.Response, error) {
	if s.clientConn == nil {
		return nil, ErrClientNotConnected
	}

	starTime := time.Now()

	respChan := make(chan *protocol.Response, 1)
	s.pendingCmdsMu.Lock()
	cmd.ReqID = s.requestIDCounter.Add(1)
	s.pendingCmds[cmd.ReqID] = respChan
	s.pendingCmdsMu.Unlock()

	cmdMsg := protocol.Message{
		ProtocolVersion: byte(protocol.Version1),
		Flags:           0x00,
		Type:            byte(protocol.MessageTypeCommand),
		Payload:         protocol.V1.EncodeCommand(cmd),
	}

	cmdData := protocol.V1.EncodeMessage(&cmdMsg)
	if err := protocol.V1.WriteFrame(s.clientConn, cmdData); err != nil {
		return nil, err
	}

	slog.Debug(
		"Sent command to server",
		slog.String("id", strconv.Itoa(int(cmd.ID))),
		slog.String("payload_size", strconv.Itoa(len(cmd.Payload))),
	)

	// Wait for the response or a timeout
	select {
	case resp := <-respChan:
		s.pendingCmdsMu.Lock()
		delete(s.pendingCmds, cmd.ReqID)
		s.pendingCmdsMu.Unlock()

		duration := time.Since(starTime)
		slog.Debug(
			"Received response from server",
			slog.String("status_code", strconv.Itoa(int(resp.Code))),
			slog.String("payload_size", strconv.Itoa(len(resp.Payload))),
			slog.Duration("duration", duration),
		)
		return resp, nil
	case <-time.After(30 * time.Second):
		s.pendingCmdsMu.Lock()
		delete(s.pendingCmds, cmd.ReqID)
		s.pendingCmdsMu.Unlock()
		return nil, ErrCommandTimeout
	}
}
