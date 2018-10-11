package coild

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/cybozu-go/coil/model"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Server keeps coild internal status.
type Server struct {
	db       model.Model
	podName  string
	nodeName string

	mu            sync.Mutex
	addressBlocks map[string][]*net.IPNet
	podIPs        map[string]net.IP

	tableID    int
	protocolID int
}

// NewServer creates a new Server.
func NewServer(db model.Model, tableID, protocolID int) *Server {
	return &Server{
		db:            db,
		addressBlocks: make(map[string][]*net.IPNet),
		podIPs:        make(map[string]net.IP),
		tableID:       tableID,
		protocolID:    protocolID,
	}
}

// Init loads status data from the database.
func (s *Server) Init(ctx context.Context) error {
	n, err := os.Hostname()
	if err != nil {
		return err
	}
	s.podName = n

	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	pod, err := clientset.CoreV1().Pods("").Get(n, metav1.GetOptions{
		IncludeUninitialized: true,
	})
	if err != nil {
		return err
	}
	s.nodeName = pod.Spec.NodeName

	// retrieve blocks acquired previously
	blocks, err := s.db.GetMyBlocks(ctx, s.nodeName)
	if err != nil {
		return err
	}
	s.addressBlocks = blocks

	// check IP address allocation status
	for poolName, v := range blocks {
		for _, block := range v {
			ips, err := s.db.GetAllocatedIPs(ctx, block)
			if err != nil {
				return err
			}

			freed := 0
			for podNSName, ip := range ips {
				sl := strings.SplitN(podNSName, "/", 2)
				if len(sl) != 2 {
					return fmt.Errorf("invalid pod ns/name: %s", podNSName)
				}
				pod, err = clientset.CoreV1().Pods(sl[0]).Get(sl[1], metav1.GetOptions{
					IncludeUninitialized: true,
				})
				if err == nil && ip.String() == pod.Status.PodIP {
					s.podIPs[podNSName] = ip
					continue
				}

				// free ip when it is not used any longer or the pod is not found.
				if err == nil || errors.IsNotFound(err) {
					err = s.db.FreeIP(ctx, block, ip)
					if err != nil {
						return err
					}
					freed++
					continue
				}
				return err
			}

			// release unused address block to the pool
			if len(ips) == freed {
				err = s.db.ReleaseBlock(ctx, s.nodeName, poolName, block)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
