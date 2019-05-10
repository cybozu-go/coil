package controller

import (
	"context"
	"fmt"

	"github.com/cybozu-go/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// Watch watches Kubernetes API server to be informed of node deletions.
// When a node is deleted from Kubernetes, the controller frees address
// blocks and IP addresses allocated to the node soon.
func (c *Controller) Watch(ctx context.Context, rev string) error {

	w, err := c.k8s.CoreV1().Nodes().Watch(metav1.ListOptions{
		Watch:           true,
		ResourceVersion: rev,
	})
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		w.Stop()
	}()

	for ev := range w.ResultChan() {
		if ev.Type != watch.Deleted {
			continue
		}

		node, ok := ev.Object.(*corev1.Node)
		if !ok {
			vk := ev.Object.GetObjectKind().GroupVersionKind()
			log.Error("unexpected object from watch", map[string]interface{}{
				"group":   vk.Group,
				"version": vk.Version,
				"kind":    vk.Kind,
			})

			return fmt.Errorf("unexpected object type: %T", ev.Object)
		}

		err := c.freeBlocks(ctx, node.Name)
		if err != nil {
			return err
		}
	}

	return nil
}
