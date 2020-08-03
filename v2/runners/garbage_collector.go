package runners

import (
	"context"
	"fmt"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// GarbageCollector is a manager.Runnable to collect
// orphaned AddressBlocks of deleted nodes.
type GarbageCollector struct {
	client.Client
	APIReader client.Reader
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Interval  time.Duration
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list

var _ manager.LeaderElectionRunnable = &GarbageCollector{}
var _ manager.Runnable = &GarbageCollector{}

// NeedLeaderElection implements manager.LeaderElectionRunnable
func (*GarbageCollector) NeedLeaderElection() bool {
	return true
}

// Start starts this runner.  This implements manager.Runnable
func (gc *GarbageCollector) Start(done <-chan struct{}) error {
	tick := time.NewTicker(gc.Interval)
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

func (gc *GarbageCollector) do(ctx context.Context) error {
	gc.Log.Info("start garbage collection")

	blocks := &coilv2.AddressBlockList{}
	if err := gc.Client.List(ctx, blocks); err != nil {
		return fmt.Errorf("failed to list address blocks: %w", err)
	}

	nodes := &corev1.NodeList{}
	if err := gc.APIReader.List(ctx, nodes); err != nil {
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

		err := gc.Client.Delete(ctx, &b)
		if err == nil {
			gc.Log.Info("deleted an orphan block", "block", b.Name, "node", n)
			continue
		}
		if apierrors.IsNotFound(err) {
			continue
		}
		return fmt.Errorf("failed to delete a block: %w", err)
	}

	return nil
}
