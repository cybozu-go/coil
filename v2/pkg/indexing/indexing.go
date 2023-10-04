package indexing

import (
	"context"
	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// SetupIndexForAddressBlock sets up an indexer for addressBlock.
func SetupIndexForAddressBlock(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &coilv2.AddressBlock{}, constants.AddressBlockRequestKey, func(rawObj client.Object) []string {
		val := rawObj.GetLabels()[constants.LabelRequest]
		if val == "" {
			return nil
		}
		return []string{val}
	})
}

// SetupIndexForPodByNodeName sets up an indexer for Pod.
func SetupIndexForPodByNodeName(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, constants.PodNodeNameKey, func(rawObj client.Object) []string {
		return []string{rawObj.(*corev1.Pod).Spec.NodeName}
	})
}
