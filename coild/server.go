package coild

import (
	"context"
	"net"
	"os"
	"sync"

	"github.com/cybozu-go/coil/model"
)

type missingEnvvar string

func (e missingEnvvar) Error() string {
	return "missing environment variable: " + string(e)
}

// Server keeps coild internal status.
type Server struct {
	db         model.Model
	tableID    int
	protocolID int

	nodeName string

	// skip routing table edit for testing
	dryRun bool

	mu            sync.Mutex
	addressBlocks map[string][]*net.IPNet
	podIPs        map[string]net.IP
}

// NewServer creates a new Server.
func NewServer(db model.Model, tableID, protocolID int) *Server {
	return &Server{
		db:            db,
		tableID:       tableID,
		protocolID:    protocolID,
		addressBlocks: make(map[string][]*net.IPNet),
		podIPs:        make(map[string]net.IP),
	}
}

// Init loads status data from the database.
func (s *Server) Init(ctx context.Context) error {
	s.nodeName = os.Getenv("COIL_NODE_NAME")
	if s.nodeName == "" {
		return missingEnvvar("COIL_NODE_NAME")
	}

	// retrieve blocks acquired previously
	blocks, err := s.db.GetMyBlocks(ctx, s.nodeName)
	if err != nil {
		return err
	}

	// check IP address allocation status
	for _, v := range blocks {
		for _, block := range v {
			ips, err := s.db.GetAllocatedIPs(ctx, block)
			if err != nil {
				return err
			}

			// In older than version 1.0.2 namespace and pod name are used for the key.
			for containerID, ip := range ips {
				s.podIPs[containerID] = ip
			}
		}
	}

	// re-retrieve blocks afer released some.
	blocks, err = s.db.GetMyBlocks(ctx, s.nodeName)
	if err != nil {
		return err
	}
	s.addressBlocks = blocks

	var flatBlocks []*net.IPNet
	for _, v := range blocks {
		for _, b := range v {
			flatBlocks = append(flatBlocks, b)
		}
	}

	err = syncRoutingTable(s.tableID, s.protocolID, flatBlocks)
	if err != nil {
		return err
	}

	return nil
}
