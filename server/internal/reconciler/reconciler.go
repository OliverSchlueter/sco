package reconciler

import (
	"log/slog"
	"time"

	"github.com/OliverSchlueter/sco-server/internal/cluster/clusterstore"
	"github.com/OliverSchlueter/sco-server/internal/node/nodestore"
)

type Reconciler struct {
	cs *clusterstore.Store
	ns *nodestore.Store
}

func New(cs *clusterstore.Store, ns *nodestore.Store) *Reconciler {
	return &Reconciler{
		cs: cs,
		ns: ns,
	}
}

func (r *Reconciler) Run() {
	c := time.Tick(time.Second * 5)
	for range c {
		slog.Debug("Starting reconciliation")
		r.reconcile()
		slog.Debug("Reconciliation done")
	}
}

func (r *Reconciler) reconcile() {
	//for _, cl := range r.cs.List() {
	//	for i, service := range cl.Services {
	//
	//	}
	//}
}
