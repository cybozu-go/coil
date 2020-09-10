package runners

import (
	"context"
	"fmt"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// NewGarbageCollector creates a manager.Runnable to collect
// orphaned AddressBlocks of deleted nodes.
func NewGarbageCollector(mgr manager.Manager, log logr.Logger, interval time.Duration) manager.Runnable {
	return &garbageCollector{
		Client:    mgr.GetClient(),
		apiReader: mgr.GetAPIReader(),
		log:       log,
		interval:  interval,
	}
}

type garbageCollector struct {
	client.Client
	apiReader client.Reader
	log       logr.Logger
	interval  time.Duration
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list

var _ manager.LeaderElectionRunnable = &garbageCollector{}

// NeedLeaderElection implements manager.LeaderElectionRunnable
func (*garbageCollector) NeedLeaderElection() bool {
	return true
}

// Start starts this runner.  This implements manager.Runnable
func (gc *garbageCollector) Start(done <-chan struct{}) error {
	tick := time.NewTicker(gc.interval)
	defer tick.Stop()

	for {
		select {
		case <-done:
			return nil
		case <-tick.C:
			if err := gc.do(context.Background()); err != nil {
				return err
			}
		}
	}
}

func (gc *garbageCollector) do(ctx context.Context) error {
	gc.log.Info("start garbage collection")

	blocks := &coilv2.AddressBlockList{}
	if err := gc.Client.List(ctx, blocks); err != nil {
		return fmt.Errorf("failed to list address blocks: %w", err)
	}

	nodes := &corev1.NodeList{}
	if err := gc.apiReader.List(ctx, nodes); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	nodeNames := make(map[string]bool)
	for _, n := range nodes.Items {
		nodeNames[n.Name] = true
	}

	for _, b := range blocks.Items {
		n := b.Labels[constants.LabelNode]
		if nodeNames[n] {
			continue
		}

		err := gc.deleteBlock(ctx, b.Name)
		if err != nil {
			return fmt.Errorf("failed to delete a block: %w", err)
		}

		gc.log.Info("deleted an orphan block", "block", b.Name, "node", n)
	}

	return nil
}

func (gc *garbageCollector) deleteBlock(ctx context.Context, name string) error {
	// remove finalizer
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		b := &coilv2.AddressBlock{}
		err := gc.apiReader.Get(ctx, client.ObjectKey{Name: name}, b)
		if err != nil {
			return client.IgnoreNotFound(err)
		}
		if !controllerutil.ContainsFinalizer(b, constants.FinCoil) {
			return nil
		}
		controllerutil.RemoveFinalizer(b, constants.FinCoil)
		return gc.Client.Update(ctx, b)
	})
	if err != nil {
		return fmt.Errorf("failed to remove finalizer from %s: %w", name, err)
	}

	// delete ignoring notfound error.
	b := &coilv2.AddressBlock{}
	b.Name = name
	return client.IgnoreNotFound(gc.Client.Delete(ctx, b))
}
