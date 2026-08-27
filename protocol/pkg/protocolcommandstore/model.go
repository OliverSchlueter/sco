package protocolcommandstore

import (
	"context"
	"net"
	"sync"
)

// ConnCtx encapsulates the context for a client connection
type ConnCtx struct {
	// ID is a unique identifier for the connection, used for tracking and logging.
	ID string

	// Conn is the underlying network connection to the client.
	Conn net.Conn

	// Ctx is the context for managing the connection's lifecycle.
	Ctx context.Context

	// LastActivity tracks the last time a message was received from the client.
	LastActivity int64

	// CustomData is a map for storing additional data related to the connection.
	customData     map[string]any
	customDataLock sync.RWMutex
}

// GetCustomData retrieves a value from the custom data map.
func (c *ConnCtx) GetCustomData(key string) (any, bool) {
	c.customDataLock.RLock()
	defer c.customDataLock.RUnlock()

	if c.customData == nil {
		return nil, false
	}

	val, found := c.customData[key]
	return val, found
}

// SetCustomData sets a value in the custom data map.
func (c *ConnCtx) SetCustomData(key string, value any) {
	c.customDataLock.Lock()
	defer c.customDataLock.Unlock()

	if c.customData == nil {
		c.customData = make(map[string]any)
	}

	c.customData[key] = value
}

// DeleteCustomData removes a value from the custom data map.
func (c *ConnCtx) DeleteCustomData(key string) {
	c.customDataLock.Lock()
	defer c.customDataLock.Unlock()

	if c.customData == nil {
		return
	}

	delete(c.customData, key)
}
