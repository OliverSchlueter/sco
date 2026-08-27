package protocolcommandstore

import "errors"

var (
	ErrCommandAlreadyRegistered = errors.New("command already registered")
	ErrCommandNotFound          = errors.New("command not found")
)
