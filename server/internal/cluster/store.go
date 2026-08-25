package cluster

import (
	"fmt"
)

type Store struct {
	clusters map[string]*Cluster
}

func NewStore() *Store {
	return &Store{
		clusters: make(map[string]*Cluster),
	}
}

func (s *Store) Get(name string) (*Cluster, error) {
	cluster, found := s.clusters[name]
	if !found {
		return nil, fmt.Errorf("cluster %q not found", name)
	}
	return cluster, nil
}

func (s *Store) List() []*Cluster {
	list := make([]*Cluster, 0, len(s.clusters))
	for _, cluster := range s.clusters {
		list = append(list, cluster)
	}
	return list
}

func (s *Store) Add(cluster *Cluster) error {
	_, found := s.clusters[cluster.Name]
	if found {
		return fmt.Errorf("cluster %q already exists", cluster.Name)
	}

	s.clusters[cluster.Name] = cluster
	return nil
}

func (s *Store) Update(cluster *Cluster) error {
	_, found := s.clusters[cluster.Name]
	if !found {
		return fmt.Errorf("cluster %q not found", cluster.Name)
	}

	s.clusters[cluster.Name] = cluster
	return nil
}

func (s *Store) Remove(name string) error {
	_, found := s.clusters[name]
	if !found {
		return fmt.Errorf("cluster %q not found", name)
	}

	delete(s.clusters, name)
	return nil
}
