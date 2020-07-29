package ipam

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/go-logr/logr"
	"github.com/willf/bitset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ErrNoBlock is an error indicating there are no available address blocks in a pool.
var ErrNoBlock = errors.New("out of blocks")

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addresspools,verbs=get;list;watch

// PoolManager manages address pools.
type PoolManager interface {
	// DropPool removes an address pool for an AddressPool, if any.
	DropPool(name string)

	// SyncPool sync address blocks of a pool.
	SyncPool(ctx context.Context, name string) error

	// AllocateBlock curves an AddressBlock out of the pool for a node.
	// If the pool runs out of the free blocks, this returns ErrNoBlock.
	AllocateBlock(ctx context.Context, poolName, nodeName string) (*coilv2.AddressBlock, error)
}

type poolManager struct {
	client client.Client
	log    logr.Logger
	scheme *runtime.Scheme

	mu    sync.Mutex
	pools map[string]*pool
}

// NewPoolManager creates a new PoolManager.
func NewPoolManager(cl client.Client, l logr.Logger, scheme *runtime.Scheme) PoolManager {
	return &poolManager{
		client: cl,
		log:    l,
		scheme: scheme,
		pools:  make(map[string]*pool),
	}
}

func (pm *poolManager) getPool(ctx context.Context, name string) (*pool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, ok := pm.pools[name]
	if !ok {
		l := pm.log.WithValues("pool", name)
		p = &pool{name: name, log: l, client: pm.client, scheme: pm.scheme}
		err := p.SyncBlocks(ctx)
		if err != nil {
			return nil, err
		}
		pm.pools[name] = p
	}

	return p, nil
}

func (pm *poolManager) DropPool(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.pools, name)
}

func (pm *poolManager) SyncPool(ctx context.Context, name string) error {
	pm.mu.Lock()
	p, ok := pm.pools[name]
	pm.mu.Unlock()

	if !ok {
		return nil
	}

	return p.SyncBlocks(ctx)
}

func (pm *poolManager) AllocateBlock(ctx context.Context, poolName, nodeName string) (*coilv2.AddressBlock, error) {
	p, err := pm.getPool(ctx, poolName)
	if err != nil {
		return nil, err
	}
	return p.AllocateBlock(ctx, nodeName)
}

// pool manages the allocation of AddressBlock CR of an AddressPool CR.
type pool struct {
	name      string
	client    client.Client
	log       logr.Logger
	scheme    *runtime.Scheme
	mu        sync.Mutex
	allocated bitset.BitSet
}

// SyncBlocks synchronizes allocated field with the current AddressBlocks.
func (p *pool) SyncBlocks(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.allocated.ClearAll()
	blocks := &coilv2.AddressBlockList{}
	err := p.client.List(ctx, blocks, client.MatchingField(constants.IndexController, p.name))
	if err != nil {
		return err
	}

	for _, b := range blocks.Items {
		p.allocated.Set(uint(b.Index))
	}

	p.log.Info("resynced block usage", "blocks", len(blocks.Items))
	return nil
}

// AllocateBlock creates an AddressBlock and returns it.
// If the pool runs out of the free blocks, this returns ErrNoBlock.
func (p *pool) AllocateBlock(ctx context.Context, nodeName string) (*coilv2.AddressBlock, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	nextIndex, ok := p.allocated.NextClear(0)
	if !ok {
		nextIndex = p.allocated.Len()
	}

	ap := &coilv2.AddressPool{}
	err := p.client.Get(ctx, client.ObjectKey{Name: p.name}, ap)
	if err != nil {
		p.log.Error(err, "failed to get AddressPool")
		return nil, err
	}
	if ap.DeletionTimestamp != nil {
		p.log.Info("unable to curve out a block because pool is under deletion")
		return nil, ErrNoBlock
	}

	var currentIndex uint
	for _, ss := range ap.Spec.Subnets {
		var ones, bits int
		if ss.IPv4 != nil {
			_, n, _ := net.ParseCIDR(*ss.IPv4) // ss was validated
			ones, bits = n.Mask.Size()
		} else {
			_, n, _ := net.ParseCIDR(*ss.IPv6) // ss was validated
			ones, bits = n.Mask.Size()
		}
		size := uint(1) << (bits - ones - int(ap.Spec.BlockSizeBits))
		if nextIndex >= (currentIndex + size) {
			currentIndex += size
			continue
		}

		ipv4, ipv6 := ss.GetBlock(nextIndex-currentIndex, int(ap.Spec.BlockSizeBits))

		r := &coilv2.AddressBlock{}
		r.Name = fmt.Sprintf("%s-%d", p.name, nextIndex)
		if err := controllerutil.SetControllerReference(ap, r, p.scheme); err != nil {
			return nil, err
		}
		r.Labels = map[string]string{
			constants.LabelPool: p.name,
			constants.LabelNode: nodeName,
		}
		controllerutil.AddFinalizer(r, constants.FinCoil)
		r.Index = int32(nextIndex)
		if ipv4 != nil {
			s := ipv4.String()
			r.IPv4 = &s
		}
		if ipv6 != nil {
			s := ipv6.String()
			r.IPv6 = &s
		}
		if err := p.client.Create(ctx, r); err != nil {
			p.log.Error(err, "failed to create AddressBlock", "index", nextIndex, "node", nodeName)
			return nil, err
		}

		p.log.Info("created AddressBlock", "index", nextIndex, "node", nodeName)
		p.allocated.Set(nextIndex)
		return r, nil
	}

	p.log.Error(ErrNoBlock, "no available blocks")
	return nil, ErrNoBlock
}

// SetupIndexer setups the required indexer for PoolManager.
func SetupIndexer(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &coilv2.AddressBlock{}, constants.IndexController, func(o runtime.Object) []string {
		job := o.(*coilv2.AddressBlock)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		return []string{owner.Name}
	})
}
