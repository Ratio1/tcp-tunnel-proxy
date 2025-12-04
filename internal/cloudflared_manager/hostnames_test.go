package cloudflaredmanager

import (
	"strings"
	"testing"
)

func TestDeriveTunnelHostnameNormalizesAndPrefixes(t *testing.T) {
	got := deriveTunnelHostname(" Db-123.Ratio1.link ")
	want := "cft-db-123-ratio1.link"
	if got != want {
		t.Fatalf("deriveTunnelHostname() = %q, want %q", got, want)
	}
}

func TestValidateHostnameRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"":                                      "empty",
		".example.com":                          "leading dot",
		"example.com.":                          "trailing dot",
		"noperiod":                              "missing dot",
		"double..dot":                           "double dot",
		"-badstart.com":                         "label starts with hyphen",
		"badend-.com":                           "label ends with hyphen",
		"bad_underscore.com":                    "invalid characters",
		strings.Repeat("a", 64) + ".example.io": "label too long",
	}

	for host, desc := range cases {
		if err := validateHostname(host); err == nil {
			t.Fatalf("validateHostname(%q) for %s: expected error", host, desc)
		}
	}
}

func TestDeriveValidatedTunnelHostname(t *testing.T) {
	host := "db-123.ratio1.link"
	got, err := deriveValidatedTunnelHostname(host)
	if err != nil {
		t.Fatalf("deriveValidatedTunnelHostname(%q) unexpected error: %v", host, err)
	}
	want := "cft-db-123-ratio1.link"
	if got != want {
		t.Fatalf("deriveValidatedTunnelHostname(%q) = %q, want %q", host, got, want)
	}
}

func TestDeriveValidatedTunnelHostnameRejectsInvalidInput(t *testing.T) {
	if _, err := deriveValidatedTunnelHostname("bad host.name"); err == nil {
		t.Fatalf("deriveValidatedTunnelHostname accepted invalid host with spaces")
	}
}
