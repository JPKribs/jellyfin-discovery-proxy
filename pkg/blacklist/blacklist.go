package blacklist

import (
	"net"
	"strings"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/logging"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// New creates a new IP blacklist from comma-separated string
// Supports both individual IPs (192.168.1.100) and CIDR notation (192.168.1.0/24)
func New(blacklistStr string) *types.IPBlacklist {
	bl := &types.IPBlacklist{
		IPs:     make(map[string]bool),
		Subnets: make([]*net.IPNet, 0),
	}

	if blacklistStr == "" {
		return bl
	}

	entries := strings.Split(blacklistStr, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Check if it's a CIDR notation (contains /)
		if strings.Contains(entry, "/") {
			_, ipnet, err := net.ParseCIDR(entry)
			if err != nil {
				logging.Logf(types.LogWarn, "Invalid CIDR notation in blacklist: %s, skipping", entry)
				continue
			}
			bl.Subnets = append(bl.Subnets, ipnet)
			logging.Logf(types.LogDebug, "Added subnet to blacklist: %s", entry)
		} else {
			// Individual IP address
			parsedIP := net.ParseIP(entry)
			if parsedIP == nil {
				logging.Logf(types.LogWarn, "Invalid IP address in blacklist: %s, skipping", entry)
				continue
			}
			bl.IPs[entry] = true
			logging.Logf(types.LogDebug, "Added IP to blacklist: %s", entry)
		}
	}

	return bl
}
