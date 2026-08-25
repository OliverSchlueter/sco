package cluster

type Endpoint struct {
	NodeID string
	Host   string
	Port   string
}

func (e *Endpoint) Address() string {
	return e.Host + ":" + e.Port
}
