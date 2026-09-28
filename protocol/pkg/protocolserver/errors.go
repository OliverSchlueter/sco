package protocolserver

import "errors"

var (
	ErrCommandTimeout     = errors.New("command timed out")
	ErrClientNotConnected = errors.New("client not connected")
	ErrConnectionNotFound = errors.New("connection not found")
	ErrConnectionClosed   = errors.New("connection closed")
)
