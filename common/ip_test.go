package common

import (
	"net"
	"reflect"
	"testing"
)

func TestSplitIPListAcceptsCommaSemicolonAndNewline(t *testing.T) {
	got := SplitIPList(" 192.168.1.1,10.0.0.0/8\n172.16.0.1; 192.168.1.1 ")
	want := []string{"192.168.1.1", "10.0.0.0/8", "172.16.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitIPList() = %#v, want %#v", got, want)
	}
}

func TestIsIpInCIDRListMatchesSingleIPAndCIDR(t *testing.T) {
	list := []string{"192.168.1.1", "10.0.0.0/8"}
	if !IsIpInCIDRList(net.ParseIP("192.168.1.1"), list) {
		t.Fatal("expected single IP to match")
	}
	if !IsIpInCIDRList(net.ParseIP("10.1.2.3"), list) {
		t.Fatal("expected CIDR range to match")
	}
	if IsIpInCIDRList(net.ParseIP("172.16.0.1"), list) {
		t.Fatal("did not expect unrelated IP to match")
	}
}
