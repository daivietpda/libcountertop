package dhcp

import (
	"net"
	"testing"
)

func TestKeepingTheAddressARenewalReturns(t *testing.T) {
	have := net.IPv4(10, 100, 101, 106)

	if !keeping(have, 24, net.IPv4(10, 100, 101, 106), 24) {
		t.Error("the address the lease returned was not kept")
	}
}

func TestKeepingDropsAnAddressTheLeaseDidNotReturn(t *testing.T) {
	if keeping(net.IPv4(10, 100, 101, 5), 24, net.IPv4(10, 100, 101, 106), 24) {
		t.Error("an address the lease did not return was kept")
	}
}

func TestKeepingDropsADifferentPrefix(t *testing.T) {
	have := net.IPv4(10, 100, 101, 106)

	if keeping(have, 16, have, 24) {
		t.Error("the same address on a wider prefix was kept")
	}
}

func TestKeepingNothingWhenThereIsNothingToKeep(t *testing.T) {
	if keeping(net.IPv4(10, 100, 101, 106), 24, nil, 0) {
		t.Error("an address was kept when the lease was being released")
	}
	if keeping(nil, 24, net.IPv4(10, 100, 101, 106), 24) {
		t.Error("an address with no value was kept")
	}
}

func TestKeepingMatchesEitherSpelling(t *testing.T) {
	four := net.IPv4(10, 100, 101, 106).To4()
	sixteen := net.IPv4(10, 100, 101, 106).To16()

	if !keeping(four, 24, sixteen, 24) {
		t.Error("the same address written two ways did not match")
	}
}

func TestCleanDomainDropsTheTrailingNUL(t *testing.T) {
	cases := map[string]string{
		"lan.example.com\x00": "lan.example.com",
		"example.com":         "example.com",
		" example.com \x00x":  "example.com",
		"\x00":                "",
		"":                    "",
	}
	for in, want := range cases {
		if got := cleanDomain(in); got != want {
			t.Errorf("cleanDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
