package controller

import (
	"context"

	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Controller is the controller service.
type Controller struct {
	model model.Model
	k8s   *kubernetes.Clientset
}

// NewController creates Controller
func NewController(m model.Model) (*Controller, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Controller{m, clientset}, nil
}

func (c *Controller) freeBlocks(ctx context.Context, name string) error {
	blockMap, err := c.model.GetMyBlocks(ctx, name)
	if err != nil {
		return err
	}
	if len(blockMap) == 0 {
		return nil
	}

	log.Info("freeing resources of a deleted node", map[string]interface{}{
		"node": name,
	})

	for poolName, blocks := range blockMap {
		for _, block := range blocks {
			err := c.model.ReleaseBlock(ctx, name, poolName, block, true)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
