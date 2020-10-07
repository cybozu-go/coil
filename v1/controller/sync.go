package controller

import (
	"context"

	"github.com/cybozu-go/coil/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func allNodeNames(ctx context.Context, m model.Model) (map[string]bool, error) {
	pools, err := m.ListPools(ctx)
	if err != nil {
		return nil, err
	}

	nodeNames := make(map[string]bool)

	for name, pool := range pools {
		for _, subnet := range pool.Subnets {
			ba, err := m.GetAssignments(ctx, name, subnet)
			if err != nil {
				return nil, err
			}

			for k := range ba.Nodes {
				nodeNames[k] = true
			}
		}
	}

	return nodeNames, nil
}

// Sync retrieves all existing Node from Kubernetes API server then
// look for nodes that exist only in the coil database.  Address
// blocks and IP addresses allocated to such deleted nodes are
// then released.
//
// The function returns ResourceVersion rev for use in Watch().
func (c *Controller) Sync(ctx context.Context) (rev string, err error) {
	names, err := allNodeNames(ctx, c.model)
	if err != nil {
		return "", err
	}

	nl, err := c.k8s.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, node := range nl.Items {
		delete(names, node.Name)
	}

	for name := range names {
		err := c.freeBlocks(ctx, name)
		if err != nil {
			return "", err
		}
	}

	return nl.ResourceVersion, nil
}
