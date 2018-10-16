package coild

import (
	"fmt"
	"strings"

	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func makeEndpoints(subsets []corev1.EndpointSubset) []string {
	eps := make(map[string]struct{})

	for _, subset := range subsets {
		for _, a := range subset.Addresses {
			for _, p := range subset.Ports {
				endpoint := fmt.Sprintf("%s:%d", a.IP, p.Port)
				eps[endpoint] = struct{}{}
			}
		}
	}

	ret := make([]string, 0, len(eps))
	for k := range eps {
		ret = append(ret, k)
	}
	return ret
}

// ResolveEtcdEndpoints checks if an endpoint begins with "@".
// If such an endpoint exists, it looks for Endpoints resource
// in "kube-system" namespace.
//
// Suppose an endpoint is "@myetcd".  If `kube-system/myetcd`
// Endpoints exists, then endpoints in cfg is replaced with
// those defined in the Endpoints resource.
func ResolveEtcdEndpoints(cfg *etcdutil.Config) error {
	if len(cfg.Endpoints) != 1 {
		return nil
	}
	e0 := cfg.Endpoints[0]
	if !strings.HasPrefix(e0, "@") {
		return nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	ep, err := clientset.CoreV1().Endpoints("kube-system").Get(e0[1:], metav1.GetOptions{})
	if err != nil {
		return err
	}

	cfg.Endpoints = makeEndpoints(ep.Subsets)
	log.Info("resolved etcd endpoints", map[string]interface{}{
		"endpoints": cfg.Endpoints,
	})
	return nil
}
