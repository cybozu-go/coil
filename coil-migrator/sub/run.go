package sub

import (
	"context"
	"fmt"

	v2 "github.com/cybozu-go/coil/v2/api/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scheme = runtime.NewScheme()

func init() {
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := v2.AddToScheme(scheme); err != nil {
		panic(err)
	}
}

func getClient() (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	ns := &corev1.Namespace{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, ns)
	if err != nil {
		return nil, fmt.Errorf("sanity: failed to retrieve default namespace: %w", err)
	}

	return k8sClient, nil
}
