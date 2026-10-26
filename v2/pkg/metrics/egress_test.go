package metrics

import (
	"strings"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/google/nftables/xt"

	"github.com/cybozu-go/coil/v2/pkg/nat/netfilter"
)

// nftRule returns a rule tagged with UserData as the nftables backend does.
func nftRule(tag string, packets, bytes uint64) *nftables.Rule {
	return &nftables.Rule{
		UserData: []byte(tag),
		Exprs: []expr.Any{
			&expr.Counter{Packets: packets, Bytes: bytes},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	}
}

// iptRule returns a rule tagged with a comment match as iptables-nft does.
func iptRule(tag string, packets, bytes uint64) *nftables.Rule {
	comment := xt.Comment(tag)
	return &nftables.Rule{
		Exprs: []expr.Any{
			&expr.Match{Name: "comment", Info: &comment},
			&expr.Counter{Packets: packets, Bytes: bytes},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	}
}

func TestRuleTag(t *testing.T) {
	tests := []struct {
		name string
		rule *nftables.Rule
		want string
	}{
		{"nftables", nftRule(netfilter.RuleTagInvalidDrop, 0, 0), netfilter.RuleTagInvalidDrop},
		{"iptables", iptRule(netfilter.RuleTagInvalidDrop, 0, 0), netfilter.RuleTagInvalidDrop},
		{"untagged", &nftables.Rule{Exprs: []expr.Any{&expr.Counter{}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ruleTag(tt.rule); got != tt.want {
				t.Errorf("ruleTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSumCounters(t *testing.T) {
	isInvalid := func(tag string) bool { return tag == netfilter.RuleTagInvalidDrop }
	isPeerSrc := func(tag string) bool { return strings.HasPrefix(tag, netfilter.RuleTagPeerSrcPrefix) }

	tests := []struct {
		name        string
		rules       []*nftables.Rule
		match       func(string) bool
		wantPackets uint64
		wantBytes   uint64
		wantFound   bool
	}{
		{
			name: "invalid rule is not the first one (nftables)",
			rules: []*nftables.Rule{
				nftRule(netfilter.RuleTagPeerSrcPrefix+"fou4_0a000001", 1, 10),
				nftRule(netfilter.RuleTagInvalidDrop, 2, 20),
			},
			match:       isInvalid,
			wantPackets: 2,
			wantBytes:   20,
			wantFound:   true,
		},
		{
			name: "invalid rule is not the first one (iptables)",
			rules: []*nftables.Rule{
				iptRule(netfilter.RuleTagPeerSrcPrefix+"fou4_0a000001", 1, 10),
				{Exprs: []expr.Any{&expr.Counter{Packets: 5, Bytes: 50}}},
				iptRule(netfilter.RuleTagInvalidDrop, 2, 20),
			},
			match:       isInvalid,
			wantPackets: 2,
			wantBytes:   20,
			wantFound:   true,
		},
		{
			name: "invalid rule is missing",
			rules: []*nftables.Rule{
				{Exprs: []expr.Any{&expr.Counter{Packets: 5, Bytes: 50}}},
			},
			match:     isInvalid,
			wantFound: false,
		},
		{
			name: "peer source rules are summed",
			rules: []*nftables.Rule{
				nftRule(netfilter.RuleTagInvalidDrop, 2, 20),
				nftRule(netfilter.RuleTagPeerSrcPrefix+"fou4_0a000001", 1, 10),
				nftRule(netfilter.RuleTagPeerSrcPrefix+"fou4_0a000002", 3, 30),
			},
			match:       isPeerSrc,
			wantPackets: 4,
			wantBytes:   40,
			wantFound:   true,
		},
		{
			name: "no peer source rules",
			rules: []*nftables.Rule{
				nftRule(netfilter.RuleTagInvalidDrop, 2, 20),
			},
			match:     isPeerSrc,
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packets, bytes, found := sumCounters(tt.rules, tt.match)
			if packets != tt.wantPackets || bytes != tt.wantBytes || found != tt.wantFound {
				t.Errorf("sumCounters() = (%d, %d, %v), want (%d, %d, %v)",
					packets, bytes, found, tt.wantPackets, tt.wantBytes, tt.wantFound)
			}
		})
	}
}
