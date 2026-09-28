package protocolserver

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OliverSchlueter/goutils/idgen"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
	"github.com/OliverSchlueter/sco-protocol/pkg/protocolcommandstore"
)

// connection owns the state shared by the reader, command handlers, and senders
// for one peer. Request IDs are local to each direction of each connection.
type connection struct {
	*protocolcommandstore.ConnCtx
	cancel    context.CancelFunc
	closeOnce sync.Once
	writeLock chan struct{}

	requestIDCounter atomic.Uint32
	pendingCmdsMu    sync.Mutex
	pendingCmds      map[uint32]chan *protocol.Response
}

func newConnection(conn net.Conn) *connection {
	ctx, cancel := context.WithCancel(context.Background())
	return &connection{
		ConnCtx: &protocolcommandstore.ConnCtx{
			ID:           idgen.GenerateID(16),
			Conn:         conn,
			Ctx:          ctx,
			LastActivity: time.Now().UnixMilli(),
		},
		cancel:      cancel,
		writeLock:   make(chan struct{}, 1),
		pendingCmds: make(map[uint32]chan *protocol.Response),
	}
}

func (c *connection) close() {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.Conn.Close()
		c.pendingCmdsMu.Lock()
		clear(c.pendingCmds)
		c.pendingCmdsMu.Unlock()
	})
}

// writeFrame serializes complete frames, including short writes, so commands
// and responses can be sent concurrently without interleaving their bytes.
func (c *connection) writeFrame(ctx context.Context, data []byte) error {
	select {
	case c.writeLock <- struct{}{}:
		defer func() { <-c.writeLock }()
	case <-ctx.Done():
		return ctx.Err()
	case <-c.Ctx.Done():
		return ErrConnectionClosed
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if c.Ctx.Err() != nil {
		return ErrConnectionClosed
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := c.Conn.SetWriteDeadline(deadline); err != nil {
			c.close()
			return err
		}
	}

	// Cancellation must also interrupt a blocked write. Wait for the callback
	// before resetting the deadline so it cannot affect the next writer.
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = c.Conn.SetWriteDeadline(time.Now())
		close(interrupted)
	})
	err := protocol.V1.WriteFrame(c.Conn, data)
	if !stop() {
		<-interrupted
	}
	_ = c.Conn.SetWriteDeadline(time.Time{})
	if err != nil {
		// A partial frame makes the stream unusable for subsequent requests.
		c.close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return err
}
