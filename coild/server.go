package coild

import (
	"context"
	"net"
	"sync"
)

// Model defines interfaces to access coil database.
type Model interface {
}

// Server keeps coild internal status.
type Server struct {
	db Model

	mu            sync.Mutex
	addressBlocks []*net.IPNet
	containerIPs  map[string][]net.IP
}

// NewServer creates a new Server.
func NewServer(db Model) *Server {
	return &Server{
		db:           db,
		containerIPs: make(map[string][]net.IP),
	}
}

// Init loads status data from the database.
func (s *Server) Init(ctx context.Context) error {
	return nil
}
