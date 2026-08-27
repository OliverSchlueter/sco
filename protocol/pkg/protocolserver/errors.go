package protocolserver

import "errors"

var (
	ErrCommandTimeout     = errors.New("command timed out")
	ErrClientNotConnected = errors.New("client not connected")
)
