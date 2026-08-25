package cluster

import (
	"log/slog"
	"math/rand"
)

type Cluster struct {
	Name     string     `json:"name"`
	Services []*Service `json:"services"`
}

type Service struct {
	Type                ServiceType         `json:"type"`
	Name                string              `json:"name"`
	Image               string              `json:"image"`
	Ports               map[string]string   `json:"ports"`
	MaxCPU              float32             `json:"max_cpu"`
	MaxMemory           int                 `json:"max_memory"`
	Replicas            int                 `json:"replicas"`
	LoadBalanceStrategy LoadBalanceStrategy `json:"load_balance_strategy"`

	Endpoints         map[string][]*Endpoint
	roundRobinCounter int
}

type ServiceType string

const (
	ServiceTypeTCP  = "tcp"
	ServiceTypeHTTP = "http"
	//ServiceTypeTCPTunnel  = "tcp_tunnel"
	//ServiceTypeHTTPTunnel = "http_tunnel"
)

type LoadBalanceStrategy string

const (
	LoadBalanceStrategyRandom     = "random"
	LoadBalanceStrategyRoundRobin = "round_robin"
)

func (s *Service) PickEndpoint(port string) *Endpoint {
	endpoints, found := s.Endpoints[port]
	if !found {
		slog.Error("No endpoints found for port", slog.String("port", port))
		return nil
	}

	if s.LoadBalanceStrategy == LoadBalanceStrategyRandom {
		return endpoints[rand.Int()%len(endpoints)]
	}
	if s.LoadBalanceStrategy == LoadBalanceStrategyRoundRobin {
		idx := s.roundRobinCounter % len(endpoints)
		endpoint := endpoints[idx]
		s.roundRobinCounter++
		return endpoint
	}

	slog.Error("Unknown load balance strategy", slog.String("strategy", string(s.LoadBalanceStrategy)))
	return nil
}
