package nodenet

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vishvananda/netlink"
)

// startLinkChurn inflates the link table so a dump spans many netlink messages,
// then mutates it from several goroutines to bump the rtnl generation counter
// while dumps are in flight. The returned func stops the churn and cleans up.
//
// It reproduces the condition from the review of cybozu-go/coil#369: once the
// process-wide mutex is removed, concurrent veth churn can interrupt a netlink
// dump (NLM_F_DUMP_INTR), so dumps must tolerate netlink.ErrDumpInterrupted.
func startLinkChurn(t *testing.T) func() {
	t.Helper()
	const filler = 256
	created := make([]netlink.Link, 0, filler)
	for i := 0; i < filler; i++ {
		l := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: fmt.Sprintf("coilfill%d", i)}}
		if err := netlink.LinkAdd(l); err != nil {
			t.Fatalf("LinkAdd %s: %v", l.Name, err)
		}
		created = append(created, l)
	}
	var stop atomic.Bool
	var wg sync.WaitGroup
	const churners = 8
	for g := 0; g < churners; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			l := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: fmt.Sprintf("coilchurn%d", g)}}
			for !stop.Load() {
				if err := netlink.LinkAdd(l); err != nil {
					continue
				}
				_ = netlink.LinkDel(l)
			}
		}(g)
	}
	return func() {
		stop.Store(true)
		wg.Wait()
		for _, l := range created {
			_ = netlink.LinkDel(l)
		}
	}
}

// TestDumpInterruptedRepro is a sanity check that the churn workload actually
// provokes the kernel into reporting an interrupted dump on this machine. If it
// does not, TestLookupNeverPropagatesDumpInterrupted below cannot meaningfully
// exercise the retry path, so we skip rather than fail.
func TestDumpInterruptedRepro(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("run as root")
	}
	stop := startLinkChurn(t)
	defer stop()
	for i := 0; i < 5000; i++ {
		if _, err := netlink.LinkList(); errors.Is(err, netlink.ErrDumpInterrupted) {
			t.Logf("LinkList returned ErrDumpInterrupted after %d iterations", i)
			return
		}
	}
	t.Skip("never observed ErrDumpInterrupted on this machine; raise filler/churners/iterations to exercise the retry path")
}

// TestLookupNeverPropagatesDumpInterrupted is the regression test for
// cybozu-go/coil#369. Before the fix, lookup() forwarded the raw
// netlink.ErrDumpInterrupted from netlink.LinkList() to its caller, turning a
// transient, partial dump into a spurious CNI ADD failure (or a false
// errNotFound). With retryDump in place, a lookup for a non-existent container
// must always resolve to errNotFound, even under heavy concurrent link churn.
func TestLookupNeverPropagatesDumpInterrupted(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("run as root")
	}
	stop := startLinkChurn(t)
	defer stop()
	for i := 0; i < 5000; i++ {
		_, err := lookup("does-not-exist-container", "eth0")
		if !errors.Is(err, errNotFound) {
			t.Fatalf("lookup() must return errNotFound under dump interruption, got: %v (iteration %d)", err, i)
		}
		if errors.Is(err, netlink.ErrDumpInterrupted) {
			t.Fatalf("lookup() leaked ErrDumpInterrupted to caller (iteration %d)", i)
		}
	}
}
