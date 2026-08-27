package protocolserver

import "errors"

var (
	ErrCommandTimeout = errors.New("command timed out")
)
