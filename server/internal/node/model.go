package node

import (
	"net"
)

const cpuThreshold = 0.2
const memThreshold = 0.2

type Node struct {
	ID   string
	Host string

	MaxCPU     float32
	UsedCPU    float32
	MaxMemory  int
	UsedMemory int

	conn *net.Conn
}
