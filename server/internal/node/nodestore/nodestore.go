package nodestore

import "github.com/OliverSchlueter/sco-server/internal/node"

type Store struct {
	nodes []*node.Node
}

func New() *Store {
	return &Store{}
}

func (s *Store) Get(id string) (*node.Node, error) {
	for _, n := range s.nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, nil
}

func (s *Store) List() []*node.Node {
	return s.nodes
}

func (s *Store) Add(n *node.Node) {
	s.nodes = append(s.nodes, n)
}

func (s *Store) Remove(id string) {
	for i, n := range s.nodes {
		if n.ID == id {
			s.nodes = append(s.nodes[:i], s.nodes[i+1:]...)
			return
		}
	}
}

func (s *Store) Update(n *node.Node) {
	s.Remove(n.ID)
	s.Add(n)
}
