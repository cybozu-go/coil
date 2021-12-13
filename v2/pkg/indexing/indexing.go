package indexing

import (
	"context"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
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
		return []string{val, constants.LabelRequest}
	})
}
