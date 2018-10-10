package coild

import (
	"context"
	"net"
	"os"
	"sync"

	"github.com/cybozu-go/coil/model"
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
	podIPs        map[string][]net.IP
}

// NewServer creates a new Server.
func NewServer(db model.Model) *Server {
	return &Server{
		db:            db,
		addressBlocks: make(map[string][]*net.IPNet),
		podIPs:        make(map[string][]net.IP),
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

	return nil
}
