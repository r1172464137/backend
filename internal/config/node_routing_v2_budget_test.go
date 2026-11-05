package config

import (
	"strings"
	"testing"
)

func TestRoutingV2AggregateBudgets(t *testing.T) {
	p, err := ParseNodeRouting([]byte(testV2))
	if err != nil {
		t.Fatal(err)
	}
	p.Rules = nil
	p.UserDefaults = []NodeUserDefault{}
	for i := 0; i < MaxRoutingValues/MaxRoutingValuesPerRule+1; i++ {
		r := NodeRoute{ID: "rr_33333333-3333-4333-8333-333333333333", Kind: "domain", Values: []string{"full:example.com"}, OutboundID: "direct"}
		r.UserIDs = make([]int64, MaxRoutingValuesPerRule)
		for j := range r.UserIDs {
			r.UserIDs[j] = int64(j + 1)
		}
		p.Rules = append(p.Rules, r)
	}
	if _, err = NormalizeNodeRouting(p); err == nil || !strings.Contains(err.Error(), "too many input values") {
		t.Fatalf("aggregate IDs: %v", err)
	}
}
