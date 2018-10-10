package model

import (
	"context"
	"net"

	"github.com/cybozu-go/coil"
)

// Model defines interfaces to access coil database.
type Model interface {
	// GetAllocatedIPs returns allocated IP addresses for a block
	// The return value is a map whose keys are those passed to AllocateIP().
	GetAllocatedIPs(ctx context.Context, block *net.IPNet) (map[string]net.IP, error)

	// AllocateIP allocates new IP address from AddressBlock
	// Multiple goroutines cannot use this concurrently.
	//
	// When no more IP address can be allocated in block, ErrBlockIsFull will be returned.
	AllocateIP(ctx context.Context, block *net.IPNet, key string) (net.IP, error)

	// FreeIP deletes allocated IP
	FreeIP(ctx context.Context, block *net.IPNet, ip net.IP) error

	// GetMyBlocks retrieves all acquired blocks for a node.
	// The return value is a map whose keys are pool names.
	GetMyBlocks(ctx context.Context, node string) (map[string][]*net.IPNet, error)

	// AcquireBlock acquires a block from the free list for node.
	AcquireBlock(ctx context.Context, node, poolName string) (*net.IPNet, error)

	// ReleaseBlock releases a block and returns it to the free list.
	ReleaseBlock(ctx context.Context, node, poolName string, block *net.IPNet) error

	// AddPool adds a new address pool.
	// name must match this regexp: ^[a-z][a-z0-9_.-]*$
	AddPool(ctx context.Context, name string, subnet *net.IPNet, blockSize int) error

	// AddSubnet adds a subnet to an existing pool.
	AddSubnet(ctx context.Context, name string, n *net.IPNet) error

	// GetPool gets pool
	GetPool(ctx context.Context, name string) (*coil.AddressPool, error)

	// RemovePool removes pool.
	RemovePool(ctx context.Context, name string) error
}
