package cluster

import "testing"

func TestService_PickEndpoint_ReturnsNilWhenPortMissing(t *testing.T) {
	service := &Service{
		Endpoints: map[string][]*Endpoint{
			"8080": {{NodeID: "n1", Host: "10.0.0.1", Port: "8080"}},
		},
	}

	if got := service.PickEndpoint("9090"); got != nil {
		t.Fatalf("PickEndpoint returned %v for missing port, want nil", got)
	}
}

func TestService_PickEndpoint_UsesRoundRobinOrder(t *testing.T) {
	first := &Endpoint{NodeID: "n1", Host: "10.0.0.1", Port: "8080"}
	second := &Endpoint{NodeID: "n2", Host: "10.0.0.2", Port: "8080"}
	third := &Endpoint{NodeID: "n3", Host: "10.0.0.3", Port: "8080"}

	service := &Service{
		LoadBalanceStrategy: LoadBalanceStrategyRoundRobin,
		Endpoints: map[string][]*Endpoint{
			"8080": {first, second, third},
		},
	}

	for _, want := range []*Endpoint{first, second, third, first, second} {
		if got := service.PickEndpoint("8080"); got != want {
			t.Fatalf("PickEndpoint() = %v, want %v", got, want)
		}
	}
}

func TestService_PickEndpoint_RandomStrategyReturnsKnownEndpoint(t *testing.T) {
	first := &Endpoint{NodeID: "n1", Host: "10.0.0.1", Port: "8080"}
	second := &Endpoint{NodeID: "n2", Host: "10.0.0.2", Port: "8080"}
	third := &Endpoint{NodeID: "n3", Host: "10.0.0.3", Port: "8080"}

	service := &Service{
		LoadBalanceStrategy: LoadBalanceStrategyRandom,
		Endpoints: map[string][]*Endpoint{
			"8080": {first, second, third},
		},
	}

	seen := map[*Endpoint]bool{}
	for range 100 {
		got := service.PickEndpoint("8080")
		if got == nil {
			t.Fatal("PickEndpoint() returned nil for random strategy")
		}
		if !seen[got] {
			seen[got] = true
		}
	}

	if len(seen) != 3 {
		t.Fatalf("random strategy selected %d unique endpoints, want 3", len(seen))
	}
}

func TestService_PickEndpoint_ReturnsNilForUnknownStrategy(t *testing.T) {
	service := &Service{
		LoadBalanceStrategy: LoadBalanceStrategy("invalid"),
		Endpoints: map[string][]*Endpoint{
			"8080": {{NodeID: "n1", Host: "10.0.0.1", Port: "8080"}},
		},
	}

	if got := service.PickEndpoint("8080"); got != nil {
		t.Fatalf("PickEndpoint returned %v for unknown strategy, want nil", got)
	}
}
