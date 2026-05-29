package cloudflaredmanager

import (
	"strings"
	"testing"
)

func TestNormalizeValidatedHostname(t *testing.T) {
	got, err := normalizeValidatedHostname(" Origin-123.Ratio1.Link ")
	if err != nil {
		t.Fatalf("normalizeValidatedHostname returned error: %v", err)
	}
	want := "origin-123.ratio1.link"
	if got != want {
		t.Fatalf("normalizeValidatedHostname() = %q, want %q", got, want)
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

func TestNormalizeValidatedHostnameRejectsInvalidInput(t *testing.T) {
	if _, err := normalizeValidatedHostname("bad host.name"); err == nil {
		t.Fatalf("normalizeValidatedHostname accepted invalid host with spaces")
	}
}
