package protocolcommandstore

import (
	"fmt"
	"slices"
	"sync"

	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
)

type Handler func(ctx *ConnCtx, msg *protocol.Message, cmd *protocol.Command) (*protocol.Response, error)

type Middleware func(next Handler) Handler

// Store stores the registered commands
type Store struct {
	commands    map[uint16]Handler
	middlewares []Middleware
	mu          sync.RWMutex
}

func New() *Store {
	return &Store{
		commands:    make(map[uint16]Handler),
		middlewares: make([]Middleware, 0),
	}
}

// RegisterHandler registers a command handler
// Returns an error if the command is already registered
func (s *Store) RegisterHandler(cmd uint16, handler Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.commands[cmd]; exists {
		return ErrCommandAlreadyRegistered
	}

	s.commands[cmd] = handler
	return nil
}

// RegisterMiddleware registers a middleware
func (s *Store) RegisterMiddleware(middleware Middleware) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.middlewares = append(s.middlewares, middleware)
}

// Execute executes a command and returns the response
func (s *Store) Execute(ctx *ConnCtx, msg *protocol.Message, cmd *protocol.Command) *protocol.Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

	handler, found := s.commands[cmd.ID]
	if !found {
		return &protocol.Response{
			Code:    protocol.StatusCommandNotFound,
			Payload: []byte(fmt.Sprintf("command with ID %d not found", cmd.ID)),
		}
	}

	for _, v := range slices.Backward(s.middlewares) {
		handler = v(handler)
	}

	response, err := handler(ctx, msg, cmd)
	if err != nil {
		return &protocol.Response{
			Code:    protocol.StatusInternalError,
			Payload: []byte(fmt.Sprintf("internal server error: %v", err)),
		}
	}

	return response
}
