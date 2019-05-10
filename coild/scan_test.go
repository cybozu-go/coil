package coild

import (
	"context"
	"net"
	"testing"
)

func TestScan(t *testing.T) {
	t.Parallel()
	server := testNewServer(t)

	err := server.scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	blockMap, err := server.db.GetMyBlocks(context.Background(), server.nodeName)
	if err != nil {
		t.Fatal(err)
	}
	blocks, ok := blockMap["default"]
	if !ok || len(blocks) != 1 {
		t.Error("non empty block is released")
	}

	for _, ip := range []string{"10.10.0.0", "10.10.0.1"} {
		_, modRev, err := server.db.GetAddressInfo(context.Background(), net.ParseIP(ip))
		if err != nil {
			t.Error(err)
		}

		err = server.db.FreeIP(context.Background(), blocks[0], net.ParseIP(ip), modRev)
		if err != nil {
			t.Error(err)
		}
	}

	err = server.scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	blockMap, err = server.db.GetMyBlocks(context.Background(), server.nodeName)
	if err != nil {
		t.Fatal(err)
	}
	blocks, ok = blockMap["default"]
	if ok && len(blocks) == 1 {
		t.Error("empty block still exists")
	}
}
