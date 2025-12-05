package cloudflaredmanager

import (
	"fmt"
	"regexp"
	"strings"
)

var hostnameLabelRE = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// deriveTunnelHostname deterministically maps an incoming SNI to the cloudflared hostname.
// Rule: prefix with "cft-".
func deriveTunnelHostname(sni string) string {
	normalized := strings.ToLower(strings.TrimSpace(sni))
	return "cft-" + normalized
}

// validateHostname checks basic DNS label constraints for use with cloudflared.
func validateHostname(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("hostname is empty")
	}
	if len(host) > 253 {
		return fmt.Errorf("hostname too long")
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return fmt.Errorf("hostname must not start or end with a dot")
	}
	if !strings.Contains(host, ".") {
		return fmt.Errorf("hostname must contain at least one dot")
	}
	if strings.Contains(host, "..") {
		return fmt.Errorf("hostname has empty label")
	}

	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 {
			return fmt.Errorf("hostname has empty label")
		}
		if len(label) > 63 {
			return fmt.Errorf("label %q too long", label)
		}
		if !hostnameLabelRE.MatchString(label) {
			return fmt.Errorf("label %q contains invalid characters", label)
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("label %q must not start or end with a hyphen", label)
		}
	}
	return nil
}

// deriveValidatedTunnelHostname normalizes, validates, and derives the cloudflared hostname from SNI.
func deriveValidatedTunnelHostname(sni string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(sni))
	if err := validateHostname(normalized); err != nil {
		return "", fmt.Errorf("invalid SNI %q: %w", sni, err)
	}
	derived := deriveTunnelHostname(normalized)
	if err := validateHostname(derived); err != nil {
		return "", fmt.Errorf("invalid derived hostname %q: %w", derived, err)
	}
	return derived, nil
}
