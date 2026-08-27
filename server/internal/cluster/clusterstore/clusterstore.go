package clusterstore

import (
	"fmt"
	"sync"

	"github.com/OliverSchlueter/sco-server/internal/cluster"
)

type Store struct {
	clusters map[string]*cluster.Cluster
	mu       sync.RWMutex
}

func New() *Store {
	return &Store{
		clusters: make(map[string]*cluster.Cluster),
	}
}

func (s *Store) Get(name string) (*cluster.Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cl, found := s.clusters[name]
	if !found {
		return nil, fmt.Errorf("cluster %q not found", name)
	}
	return cl, nil
}

func (s *Store) List() []*cluster.Cluster {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*cluster.Cluster, 0, len(s.clusters))
	for _, cl := range s.clusters {
		list = append(list, cl)
	}
	return list
}

func (s *Store) Add(cluster *cluster.Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, found := s.clusters[cluster.Name]
	if found {
		return fmt.Errorf("cluster %q already exists", cluster.Name)
	}

	s.clusters[cluster.Name] = cluster
	return nil
}

func (s *Store) Update(cluster *cluster.Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, found := s.clusters[cluster.Name]
	if !found {
		return fmt.Errorf("cluster %q not found", cluster.Name)
	}

	s.clusters[cluster.Name] = cluster
	return nil
}

func (s *Store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, found := s.clusters[name]
	if !found {
		return fmt.Errorf("cluster %q not found", name)
	}

	delete(s.clusters, name)
	return nil
}
