package protocolserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
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
}

func New(port string, commandStore *protocolcommandstore.Store) *Server {
	return &Server{
		port:        port,
		cs:          commandStore,
		connections: make(map[string]*protocolcommandstore.ConnCtx),
	}
}

// Start starts the server and listens for incoming connections.
// It blocks until the server is stopped.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return err
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Warn("Failed to accept connection", sloki.WrapError(err))
			continue
		}

		go s.handleConnection(conn)
	}
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

	startTime := time.Now()

	msg := protocol.GetMessageFromPool()
	defer protocol.PutMessageToPool(msg)
	if err := protocol.V1.DecodeMessageInto(frame, msg); err != nil {
		s.writeResponse(conn, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte(err.Error()),
		})

		return false
	}

	if msg.Type != byte(protocol.MessageTypeCommand) {
		s.writeResponse(conn, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte("Only command messages are allowed"),
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

	cmd := protocol.GetCommandFromPool()
	defer protocol.PutCommandToPool(cmd)
	if err := protocol.V1.DecodeCommandInto(msg, cmd); err != nil {
		s.writeResponse(conn, &protocol.Response{
			Code:    protocol.StatusInvalidMessage,
			Payload: []byte(err.Error()),
		})

		return false
	}

	resp := s.cs.Execute(ctx, msg, cmd)

	// Ensure the response has the same ReqID as the command for proper correlation on the client side
	resp.ReqID = cmd.ReqID

	s.writeResponse(conn, resp)

	// Update last activity timestamp for cleanup purposes
	ctx.LastActivity = startTime.UnixMilli()

	elapsedTime := time.Since(startTime)

	slog.Info(
		"Processed command",
		slog.String("conn_id", ctx.ID),
		slog.Int("command_id", int(cmd.ID)),
		slog.String("payload", string(cmd.Payload)),
		slog.Duration("duration", elapsedTime),
	)

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
