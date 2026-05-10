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

// Load loads configuration from environment variables.
//
// Recognized variables:
//   JELLYFIN_SERVER_URL  - URL the proxy fetches /System/Info/Public from.
//                          Default: http://localhost:8096.
//   PROXY_URL            - URL advertised to discovery clients.
//                          Default: JELLYFIN_SERVER_URL.
//   PROXY_URL_IPV6       - Optional. When set and different from PROXY_URL,
//                          the proxy emits a second discovery response
//                          carrying this URL so dual-stack clients see a
//                          v6 endpoint too.
func Load() (*types.Config, error) {
	serverURL := os.Getenv("JELLYFIN_SERVER_URL")
	if serverURL == "" {
		logging.Logln(types.LogInfo, "JELLYFIN_SERVER_URL not set, using default http://localhost:8096")
		serverURL = "http://localhost:8096"
	}

	proxyURL := os.Getenv("PROXY_URL")
	if proxyURL == "" {
		logging.Logln(types.LogInfo, "PROXY_URL not set, using JELLYFIN_SERVER_URL for the Address field")
		proxyURL = serverURL
	} else {
		logging.Logf(types.LogInfo, "PROXY_URL set to %s, will use for Address field in responses", proxyURL)
		if server.IsHostname(proxyURL) {
			logging.Logln(types.LogInfo, "PROXY_URL is a hostname, will broadcast both hostname and IP responses for non-Avahi device compatibility")
		}
	}

	proxyURLv6 := os.Getenv("PROXY_URL_IPV6")
	if proxyURLv6 != "" {
		logging.Logf(types.LogInfo, "PROXY_URL_IPV6 set to %s, will emit a second discovery response per request", proxyURLv6)
		if server.IsHostname(proxyURLv6) {
			logging.Logf(types.LogDebug, "PROXY_URL_IPV6 is a hostname: %s", proxyURLv6)
		}
	}

	serverURL = strings.TrimSuffix(serverURL, "/")
	proxyURL = strings.TrimSuffix(proxyURL, "/")
	proxyURLv6 = strings.TrimSuffix(proxyURLv6, "/")

	logging.Logf(types.LogInfo, "Target Jellyfin server: %s", serverURL)
	logging.Logf(types.LogDebug, "Resolved URLs - server: '%s', proxy: '%s', proxyV6: '%s'", serverURL, proxyURL, proxyURLv6)

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

	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
		logging.Logln(types.LogInfo, "HTTP_PORT not set, using default port 8080")
	} else {
		logging.Logf(types.LogInfo, "HTTP_PORT set to: %s", httpPort)
	}

	return &types.Config{
		ServerURL:        serverURL,
		ProxyURL:         proxyURL,
		ProxyURLv6:       proxyURLv6,
		NetworkInterface: networkInterface,
		BindIP:           bindIP,
		HTTPPort:         httpPort,
	}, nil
}
