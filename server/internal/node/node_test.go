package node

import (
	"net"
	"testing"
)

func TestNode_CanPlaceTask(t *testing.T) {
	var nilConn net.Conn = nil

	n := &Node{
		MaxCPU:     4.0,
		UsedCPU:    1.0,
		MaxMemory:  8000,
		UsedMemory: 1000,
		conn:       &nilConn, // non-nil pointer to avoid actual network ping
	}

	t.Run("succeeds when enough resources", func(t *testing.T) {
		if !n.CanPlaceTask(1.0, 1024) {
			t.Fatalf("expected CanPlaceTask to return true when resources are sufficient")
		}
	})

	t.Run("fails when CPU insufficient", func(t *testing.T) {
		if n.CanPlaceTask(3.0, 1024) {
			t.Fatalf("expected CanPlaceTask to return false when CPU is insufficient")
		}
	})

	t.Run("fails when memory insufficient", func(t *testing.T) {
		if n.CanPlaceTask(1.0, 6000) {
			t.Fatalf("expected CanPlaceTask to return false when memory is insufficient")
		}
	})

	t.Run("fails when ping errors", func(t *testing.T) {
		// Create a node that will attempt to resolve an invalid host, causing Ping to error.
		nPingFail := &Node{
			MaxCPU:     4.0,
			UsedCPU:    0.0,
			MaxMemory:  8000,
			UsedMemory: 0,
			Host:       "invalid-host-name-for-testing-should-not-resolve",
			conn:       nil, // nil -> Ping will perform resolution and fail
		}

		if nPingFail.CanPlaceTask(0.1, 128) {
			t.Fatalf("expected CanPlaceTask to return false when Ping fails")
		}
	})
}
