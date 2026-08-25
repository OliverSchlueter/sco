package node

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// CanPlaceTask returns true if the node can place a task with the given CPU and memory requirements.
// cpu is in cores.
// mem is in bytes.
func (n *Node) CanPlaceTask(cpu float32, mem int) bool {
	if err := n.Ping(); err != nil {
		return false
	}

	availableCPU := n.MaxCPU - (n.MaxCPU * cpuThreshold) - n.UsedCPU
	availableMemory := n.MaxMemory - int(float32(n.MaxMemory)*memThreshold) - n.UsedMemory

	return availableCPU >= cpu && availableMemory >= mem
}

func (n *Node) Ping() error {
	if n.conn != nil {
		// TODO: send ping command (see protocol) and check if the node is still alive
		return nil
	}

	// Resolve address
	dst, err := net.ResolveIPAddr("ip4", n.Host)
	if err != nil {
		return fmt.Errorf("resolve address failed: %w", err)
	}

	// Listen for ICMP packets
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return fmt.Errorf("listen icmp packet failed: %w", err)
	}
	defer conn.Close()

	// Create ICMP Echo Request
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1,
			Data: []byte("PING"),
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return fmt.Errorf("marshal icmp packet failed: %w", err)
	}

	start := time.Now()

	// Send packet
	_, err = conn.WriteTo(msgBytes, dst)
	if err != nil {
		return fmt.Errorf("write icmp packet failed: %w", err)
	}

	// Set timeout
	err = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err != nil {
		return fmt.Errorf("set read deadline failed: %w", err)
	}

	reply := make([]byte, 1500)

	// Receive reply
	count, peer, err := conn.ReadFrom(reply)
	if err != nil {
		return fmt.Errorf("read icmp packet failed: %w", err)
	}

	duration := time.Since(start)

	// Parse ICMP message
	rm, err := icmp.ParseMessage(ipv4.ICMPTypeEchoReply.Protocol(), reply[:count])
	if err != nil {
		return fmt.Errorf("parse icmp message failed: %w", err)
	}

	switch rm.Type {
	case ipv4.ICMPTypeEchoReply:
		slog.Debug(
			"Ping success",
			slog.String("host", peer.String()),
			slog.Duration("duration", duration),
		)
		return nil
	default:
		return fmt.Errorf("unknown icmp type: %d", rm.Type)
	}
}
