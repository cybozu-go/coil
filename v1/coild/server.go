package coild

import (
	"context"
	"net"
	"os"

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
}

// NewServer creates a new Server.
func NewServer(db model.Model, tableID, protocolID int) *Server {
	return &Server{
		db:         db,
		tableID:    tableID,
		protocolID: protocolID,
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
