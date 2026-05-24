package model

import (
	"net"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestTokenGetModelLimitsNormalizesDelimitedValues(t *testing.T) {
	token := &Token{
		ModelLimits: " gpt-5.5, ,gpt-4o\nclaude-sonnet-4\r\n gpt-5.5 ",
	}

	limits := token.GetModelLimits()
	expected := []string{"gpt-5.5", "gpt-4o", "claude-sonnet-4"}
	if len(limits) != len(expected) {
		t.Fatalf("expected %d model limits, got %d: %#v", len(expected), len(limits), limits)
	}
	for i, expectedLimit := range expected {
		if limits[i] != expectedLimit {
			t.Fatalf("expected model limit %d to be %q, got %q", i, expectedLimit, limits[i])
		}
	}

	limitMap := token.GetModelLimitsMap()
	for _, expectedLimit := range expected {
		if !limitMap[expectedLimit] {
			t.Fatalf("expected model limit map to contain %q, got %#v", expectedLimit, limitMap)
		}
	}
	if limitMap[""] || limitMap[" "] {
		t.Fatalf("expected empty model limits to be ignored, got %#v", limitMap)
	}
}

func TestTokenGetIpLimitsAcceptsCommaAndNewlineDelimitedValues(t *testing.T) {
	allowIps := "127.0.0.1, 10.0.0.0/8\n 192.168.1.1\r\n"
	token := &Token{AllowIps: &allowIps}

	limits := token.GetIpLimits()
	expected := []string{"127.0.0.1", "10.0.0.0/8", "192.168.1.1"}
	if len(limits) != len(expected) {
		t.Fatalf("expected %d IP limits, got %d: %#v", len(expected), len(limits), limits)
	}
	for i, expectedLimit := range expected {
		if limits[i] != expectedLimit {
			t.Fatalf("expected IP limit %d to be %q, got %q", i, expectedLimit, limits[i])
		}
	}
}

func TestTokenGetIpLimitsCanBeUsedForIpWhitelistEnforcement(t *testing.T) {
	allowIps := "127.0.0.1, 10.0.0.0/8"
	token := &Token{AllowIps: &allowIps}
	limits := token.GetIpLimits()

	if !common.IsIpInCIDRList(net.ParseIP("127.0.0.1"), limits) {
		t.Fatalf("expected exact IP to match parsed allowlist %#v", limits)
	}
	if !common.IsIpInCIDRList(net.ParseIP("10.2.3.4"), limits) {
		t.Fatalf("expected CIDR IP to match parsed allowlist %#v", limits)
	}
	if common.IsIpInCIDRList(net.ParseIP("8.8.8.8"), limits) {
		t.Fatalf("expected non-allowlisted IP not to match parsed allowlist %#v", limits)
	}
}
