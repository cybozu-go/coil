package ipam

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// DefaultAllocTimeout is the default timeout duration for NodeIPAM.Allocate
const DefaultAllocTimeout = 10 * time.Second

type allocInfo struct {
	IPv4      net.IP
	IPv6      net.IP
	Pool      *nodePool
	BlockName string
	Index     uint
}

func allocKey(containerID, iface string) string {
	return fmt.Sprintf("%s:%s", containerID, iface)
}

// NodeIPAM manages IP address assignments to Pods on each node.
type NodeIPAM interface {
	// Register registers previously allocated IP addresses.
	Register(ctx context.Context, poolName, containerID, iface string, ipv4, ipv6 net.IP) error

	// GC returns unused address blocks to the pool.
	//
	// This method is intended to be called once during the startup
	// and just after all existing containers are registered.
	GC(ctx context.Context) error

	// Allocate allocates IP addresses for `(containerID, iface)` from the pool.
	//
	// Allocate may timeout.  The default timeout duration is DefaultAllocTimeout.
	// To specify shorter duration, pass `ctx` with timeout.
	// https://golang.org/pkg/context/#WithTimeout
	//
	// To test whether the returned error came from the timeout, do
	// `errors.Is(err, context.DeadlineExceeded)`.
	Allocate(ctx context.Context, poolName, containerID, iface string) (ipv4, ipv6 net.IP, err error)

	// Free frees the addresses allocated for `(containerID, iface)`.
	//
	// If no IP address has been allocated, this returns `nil`.
	//
	// non-nil error is returned only when it fails to return an unused
	// AddressBlock to the pool.
	Free(ctx context.Context, containerID, iface string) error

	// Notify notifies a goroutine waiting for BlockRequest completion
	Notify(poolName string)
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks,verbs=get;list;delete
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=blockrequests,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=blockrequests/status,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get

type nodeIPAM struct {
	nodeName  string
	log       logr.Logger
	client    client.Client
	apiReader client.Reader
	scheme    *runtime.Scheme
	exporter  nodenet.RouteExporter

	mu    sync.Mutex
	pools map[string]*nodePool
	node  *corev1.Node

	allocInfoMap sync.Map
}

// NewNodeIPAM creates a new NodeIPAM object.
//
// If `exporter` is non-nil, this calls `exporter.Sync` to
// add or delete routes when it allocate or delete AddressBlocks.
func NewNodeIPAM(nodeName string, l logr.Logger, mgr manager.Manager, exporter nodenet.RouteExporter) NodeIPAM {
	return &nodeIPAM{
		nodeName:  nodeName,
		log:       l,
		client:    mgr.GetClient(),
		apiReader: mgr.GetAPIReader(),
		scheme:    mgr.GetScheme(),
		exporter:  exporter,
		pools:     make(map[string]*nodePool),
	}
}

func (n *nodeIPAM) sync(ctx context.Context) error {
	if n.exporter == nil {
		return nil
	}

	blocks := &coilv2.AddressBlockList{}
	if err := n.apiReader.List(ctx, blocks, client.MatchingLabels{
		constants.LabelNode: n.nodeName,
	}); err != nil {
		return err
	}

	var subnets []*net.IPNet
	for _, block := range blocks.Items {
		if block.IPv4 != nil {
			_, n, _ := net.ParseCIDR(*block.IPv4)
			subnets = append(subnets, n)
		}
		if block.IPv6 != nil {
			_, n, _ := net.ParseCIDR(*block.IPv6)
			subnets = append(subnets, n)
		}
	}
	return n.exporter.Sync(subnets)

}

func (n *nodeIPAM) Register(ctx context.Context, poolName, containerID, iface string, ipv4, ipv6 net.IP) error {
	p, err := n.getPool(ctx, poolName)
	if err != nil {
		return err
	}

	ai := p.register(containerID, iface, ipv4, ipv6)
	if ai != nil {
		n.allocInfoMap.Store(allocKey(containerID, iface), ai)
	}
	return nil
}

func (n *nodeIPAM) GC(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, np := range n.pools {
		if err := np.gc(ctx); err != nil {
			return err
		}
	}
	return n.sync(ctx)
}

func (n *nodeIPAM) Allocate(ctx context.Context, poolName, containerID, iface string) (ipv4, ipv6 net.IP, err error) {
	key := allocKey(containerID, iface)
	if val, ok := n.allocInfoMap.Load(key); ok {
		val := val.(*allocInfo)
		return val.IPv4, val.IPv6, nil
	}

	p, err := n.getPool(ctx, poolName)
	if err != nil {
		return nil, nil, err
	}
	ai, toSync, err := p.allocate(ctx)
	if err != nil {
		return nil, nil, err
	}
	if toSync {
		if err := n.sync(ctx); err != nil {
			return nil, nil, err
		}
	}
	n.allocInfoMap.Store(key, ai)
	return ai.IPv4, ai.IPv6, nil
}

func (n *nodeIPAM) Free(ctx context.Context, containerID, iface string) error {
	key := allocKey(containerID, iface)
	val, ok := n.allocInfoMap.Load(key)
	if !ok {
		return nil
	}

	ai := val.(*allocInfo)
	toSync, err := ai.Pool.free(ctx, ai.BlockName, ai.Index)
	if err != nil {
		return err
	}
	if toSync {
		if err := n.sync(ctx); err != nil {
			return err
		}
	}
	n.allocInfoMap.Delete(key)
	return nil
}

func (n *nodeIPAM) Notify(poolName string) {
	n.mu.Lock()
	p, ok := n.pools[poolName]
	n.mu.Unlock()

	if ok {
		p.Notify()
	}
}

func (n *nodeIPAM) getPool(ctx context.Context, name string) (*nodePool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.node == nil {
		node := &corev1.Node{}
		if err := n.apiReader.Get(ctx, client.ObjectKey{Name: n.nodeName}, node); err != nil {
			return nil, fmt.Errorf("failed to get Node resource: %w", err)
		}
		n.node = node
	}

	p, ok := n.pools[name]
	if !ok {
		p = &nodePool{
			poolName:            name,
			nodeName:            n.nodeName,
			node:                n.node,
			log:                 n.log.WithValues("pool", name),
			client:              n.client,
			apiReader:           n.apiReader,
			scheme:              n.scheme,
			requestCompletionCh: make(chan struct{}),
			blockAlloc:          make(map[string]allocator),
		}
		if err := p.syncBlock(ctx); err != nil {
			return nil, err
		}
		n.pools[name] = p
	}

	return p, nil
}

type nodePool struct {
	poolName  string
	nodeName  string
	node      *corev1.Node
	log       logr.Logger
	client    client.Client
	apiReader client.Reader
	scheme    *runtime.Scheme

	requestCompletionCh chan struct{}

	mu         sync.Mutex
	blockAlloc map[string]allocator
}

// syncBlock synchronizes address block information.
func (p *nodePool) syncBlock(ctx context.Context) error {
	blocks := &coilv2.AddressBlockList{}
	err := p.apiReader.List(ctx, blocks, client.MatchingLabels{
		constants.LabelPool: p.poolName,
		constants.LabelNode: p.nodeName,
	})
	if err != nil {
		return err
	}

	for _, block := range blocks.Items {
		if _, ok := p.blockAlloc[block.Name]; ok {
			continue
		}

		p.log.Info("adding a new block",
			"name", block.Name,
			"block-pool", block.Labels[constants.LabelPool],
			"block-node", block.Labels[constants.LabelNode],
		)
		p.blockAlloc[block.Name] = newAllocator(block.IPv4, block.IPv6)
	}
	return nil
}

func (p *nodePool) deleteBlock(ctx context.Context, name string) error {
	// remove finalizer
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		b := &coilv2.AddressBlock{}
		err := p.apiReader.Get(ctx, client.ObjectKey{Name: name}, b)
		if err != nil {
			return client.IgnoreNotFound(err)
		}
		if !controllerutil.ContainsFinalizer(b, constants.FinCoil) {
			return nil
		}
		controllerutil.RemoveFinalizer(b, constants.FinCoil)
		return p.client.Update(ctx, b)
	})
	if err != nil {
		return fmt.Errorf("failed to remove finalizer from %s: %w", name, err)
	}

	// delete ignoring notfound error.
	b := &coilv2.AddressBlock{}
	b.Name = name
	return client.IgnoreNotFound(p.client.Delete(ctx, b))
}

func (p *nodePool) gc(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.syncBlock(ctx); err != nil {
		return err
	}

	for name, alloc := range p.blockAlloc {
		if !alloc.isEmpty() {
			continue
		}

		p.log.Info("freeing an unused block", "block", name)
		if err := p.deleteBlock(ctx, name); err != nil {
			return err
		}
		delete(p.blockAlloc, name)
	}

	return nil
}

// Notify notifies a goroutine waiting for BlockRequest completion
func (p *nodePool) Notify() {
	select {
	case p.requestCompletionCh <- struct{}{}:
	default:
	}
}

func (p *nodePool) register(containerID, iface string, ipv4, ipv6 net.IP) *allocInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	for block, alloc := range p.blockAlloc {
		if idx, ok := alloc.register(ipv4, ipv6); ok {
			p.log.Info("registered existing IP",
				"block", block,
				"container", containerID,
				"iface", iface,
				"idx", idx,
			)
			return &allocInfo{
				IPv4:      ipv4,
				IPv6:      ipv6,
				BlockName: block,
				Index:     idx,
				Pool:      p,
			}
		}
	}

	p.log.Info("warn: failed to register IP",
		"container", containerID,
		"iface", iface,
		"ipv4", ipv4.String(),
		"ipv6", ipv6.String(),
	)
	return nil
}

func (p *nodePool) allocateFrom(alloc allocator, block string, toSync bool) (*allocInfo, bool, error) {
	ipv4, ipv6, idx, ok := alloc.allocate()
	if !ok {
		panic("bug")
	}

	p.log.Info("allocated",
		"block", block,
		"ipv4", ipv4, "ipv6", ipv6,
	)
	return &allocInfo{
		IPv4:      ipv4,
		IPv6:      ipv6,
		BlockName: block,
		Index:     idx,
		Pool:      p,
	}, toSync, nil
}

func (p *nodePool) allocate(ctx context.Context) (*allocInfo, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for block, alloc := range p.blockAlloc {
		if alloc.isFull() {
			continue
		}

		return p.allocateFrom(alloc, block, false)
	}

	p.log.Info("requesting a new block")
	ctx, cancel := context.WithTimeout(ctx, DefaultAllocTimeout)
	defer cancel()

	req := &coilv2.BlockRequest{}
	req.Name = fmt.Sprintf("req-%s-%s", p.poolName, p.nodeName)

	// delete existing request, if any
	err := p.client.Delete(ctx, req)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, false, fmt.Errorf("failed to delete existing BlockRequest: %w", err)
	}

	if err := controllerutil.SetOwnerReference(p.node, req, p.scheme); err != nil {
		return nil, false, fmt.Errorf("failed to set owner reference: %w", err)
	}
	req.Spec.NodeName = p.nodeName
	req.Spec.PoolName = p.poolName
	if err := p.client.Create(ctx, req); err != nil {
		return nil, false, fmt.Errorf("failed to create BlockRequest: %w", err)
	}

	p.log.Info("waiting for request completion")
	select {
	case <-ctx.Done():
		return nil, false, fmt.Errorf("aborting new block request: %w", ctx.Err())
	case <-p.requestCompletionCh:
	}
	if err := p.client.Get(ctx, client.ObjectKey{Name: req.Name}, req); err != nil {
		return nil, false, fmt.Errorf("failed to get the request: %w", err)
	}
	block, err := req.GetResult()
	if err != nil {
		return nil, false, err
	}

	if err := p.syncBlock(ctx); err != nil {
		return nil, false, fmt.Errorf("failed to sync blocks: %w", err)
	}
	alloc, ok := p.blockAlloc[block]
	if !ok {
		panic("bug")
	}
	return p.allocateFrom(alloc, block, true)
}

func (p *nodePool) free(ctx context.Context, blockName string, idx uint) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	alloc, ok := p.blockAlloc[blockName]
	if !ok {
		panic("bug")
	}
	alloc.free(idx)
	if !alloc.isEmpty() {
		return false, nil
	}

	p.log.Info("freeing an empty block", "block", blockName)
	if err := p.deleteBlock(ctx, blockName); err != nil {
		return false, fmt.Errorf("failed to free block %s: %w", blockName, err)
	}
	delete(p.blockAlloc, blockName)
	return true, nil
}
