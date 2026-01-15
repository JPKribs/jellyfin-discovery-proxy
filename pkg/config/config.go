package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/logging"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/server"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// Load loads configuration from environment variables
func Load() (*types.Config, error) {
	// Get server URLs from environment variables
	serverURL := os.Getenv("JELLYFIN_SERVER_URL")
	serverURLv4 := os.Getenv("JELLYFIN_SERVER_URL_IPV4")
	serverURLv6 := os.Getenv("JELLYFIN_SERVER_URL_IPV6")

	// Determine which URLs to use
	if serverURLv4 == "" && serverURLv6 == "" {
		// Legacy mode: use JELLYFIN_SERVER_URL for both
		if serverURL == "" {
			logging.Logln(types.LogInfo, "No server URL environment variables set, using default http://localhost:8096")
			serverURL = "http://localhost:8096"
		}
		serverURLv4 = serverURL
		serverURLv6 = serverURL
		logging.Logf(types.LogDebug, "Using legacy mode - both IPv4 and IPv6 will use: '%s'", serverURL)
	} else {
		// New mode: use separate URLs for IPv4 and IPv6
		if serverURLv4 == "" && serverURLv6 == "" {
			return nil, fmt.Errorf("at least one of JELLYFIN_SERVER_URL_IPV4 or JELLYFIN_SERVER_URL_IPV6 must be set")
		}
		if serverURLv4 == "" {
			serverURLv4 = serverURLv6
			logging.Logf(types.LogInfo, "JELLYFIN_SERVER_URL_IPV4 not set, using IPv6 URL for IPv4: %s", serverURLv4)
		}
		if serverURLv6 == "" {
			serverURLv6 = serverURLv4
			logging.Logf(types.LogInfo, "JELLYFIN_SERVER_URL_IPV6 not set, using IPv4 URL for IPv6: %s", serverURLv6)
		}
		logging.Logf(types.LogDebug, "IPv4 server URL: '%s'", serverURLv4)
		logging.Logf(types.LogDebug, "IPv6 server URL: '%s'", serverURLv6)
	}

	// Get proxy URL from environment variable
	proxyURL := os.Getenv("PROXY_URL")
	proxyURLv4 := os.Getenv("PROXY_URL_IPV4")
	proxyURLv6 := os.Getenv("PROXY_URL_IPV6")

	// Determine which proxy URLs to use
	if proxyURLv4 == "" && proxyURLv6 == "" {
		// Legacy mode: use PROXY_URL for both
		if proxyURL == "" {
			logging.Logln(types.LogInfo, "PROXY_URL environment variable not set, will use JELLYFIN_SERVER_URL for Address field")
			proxyURLv4 = serverURLv4
			proxyURLv6 = serverURLv6
		} else {
			proxyURLv4 = proxyURL
			proxyURLv6 = proxyURL
			logging.Logf(types.LogInfo, "PROXY_URL set to %s, will use for Address field in responses", proxyURL)
			logging.Logf(types.LogDebug, "Using legacy proxy URL mode - both IPv4 and IPv6 will use: '%s'", proxyURL)
			if server.IsHostname(proxyURL) {
				logging.Logln(types.LogInfo, "PROXY_URL is a hostname, will broadcast both hostname and IP responses for non-Avahi device compatibility")
				logging.Logf(types.LogDebug, "Detected hostname in PROXY_URL: %s", proxyURL)
			}
		}
	} else {
		// New mode: use separate proxy URLs
		if proxyURLv4 == "" {
			proxyURLv4 = serverURLv4
			logging.Logf(types.LogDebug, "PROXY_URL_IPV4 not set, using server URL: %s", proxyURLv4)
		}
		if proxyURLv6 == "" {
			proxyURLv6 = serverURLv6
			logging.Logf(types.LogDebug, "PROXY_URL_IPV6 not set, using server URL: %s", proxyURLv6)
		}
		logging.Logf(types.LogInfo, "IPv4 proxy URL: %s", proxyURLv4)
		logging.Logf(types.LogInfo, "IPv6 proxy URL: %s", proxyURLv6)
		if server.IsHostname(proxyURLv4) {
			logging.Logf(types.LogDebug, "IPv4 proxy URL is a hostname: %s", proxyURLv4)
		}
		if server.IsHostname(proxyURLv6) {
			logging.Logf(types.LogDebug, "IPv6 proxy URL is a hostname: %s", proxyURLv6)
		}
	}

	// Remove trailing slash if present from URLs
	serverURLv4 = strings.TrimSuffix(serverURLv4, "/")
	serverURLv6 = strings.TrimSuffix(serverURLv6, "/")
	proxyURLv4 = strings.TrimSuffix(proxyURLv4, "/")
	proxyURLv6 = strings.TrimSuffix(proxyURLv6, "/")
	logging.Logf(types.LogDebug, "Processed IPv4 - serverURL: '%s', proxyURL: '%s'", serverURLv4, proxyURLv4)
	logging.Logf(types.LogDebug, "Processed IPv6 - serverURL: '%s', proxyURL: '%s'", serverURLv6, proxyURLv6)

	logging.Logf(types.LogInfo, "Target Jellyfin server IPv4: %s", serverURLv4)
	logging.Logf(types.LogInfo, "Target Jellyfin server IPv6: %s", serverURLv6)

	// Get network interface if specified
	networkInterface := os.Getenv("NETWORK_INTERFACE")
	var bindIP string
	if networkInterface != "" {
		logging.Logf(types.LogInfo, "NETWORK_INTERFACE set to: %s", networkInterface)
		iface, err := net.InterfaceByName(networkInterface)
		if err != nil {
			return nil, fmt.Errorf("failed to find network interface '%s': %v", networkInterface, err)
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("failed to get addresses for interface '%s': %v", networkInterface, err)
		}

		// Find first IPv4 address
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					bindIP = ipnet.IP.String()
					logging.Logf(types.LogInfo, "Binding to interface %s with IP: %s", networkInterface, bindIP)
					break
				}
			}
		}

		if bindIP == "" {
			return nil, fmt.Errorf("no IPv4 address found on interface '%s'", networkInterface)
		}
	} else {
		bindIP = "0.0.0.0"
		logging.Logln(types.LogInfo, "No NETWORK_INTERFACE specified, binding to all interfaces")
	}

	// Get HTTP port from environment variable (default to 8080)
	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
		logging.Logln(types.LogInfo, "HTTP_PORT not set, using default port 8080")
	} else {
		logging.Logf(types.LogInfo, "HTTP_PORT set to: %s", httpPort)
	}

	return &types.Config{
		ServerURLv4:      serverURLv4,
		ServerURLv6:      serverURLv6,
		ProxyURLv4:       proxyURLv4,
		ProxyURLv6:       proxyURLv6,
		NetworkInterface: networkInterface,
		BindIP:           bindIP,
		HTTPPort:         httpPort,
	}, nil
}
