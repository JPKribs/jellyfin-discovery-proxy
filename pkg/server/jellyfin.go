package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/logging"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// FetchInfo retrieves server information from Jellyfin System/Info Endpoint
func FetchInfo(serverURL string) (*types.SystemInfoResponse, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	logging.Logf(types.LogDebug, "Created HTTP client with timeout: 5s")

	// Call the Jellyfin system info endpoint
	infoURL := fmt.Sprintf("%s/System/Info/Public", serverURL)
	logging.Logf(types.LogInfo, "Fetching server info from: %s", infoURL)
	logging.Logf(types.LogDebug, "Making HTTP GET request to: %s", infoURL)

	resp, err := client.Get(infoURL)
	if err != nil {
		logging.Logf(types.LogDebug, "HTTP request error type: %T", err)
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	logging.Logf(types.LogDebug, "HTTP response status: %d %s", resp.StatusCode, resp.Status)
	logging.Logf(types.LogDebug, "HTTP response headers: %v", resp.Header)

	if resp.StatusCode != http.StatusOK {
		logging.Logf(types.LogDebug, "Non-OK status code received: %d", resp.StatusCode)
		return nil, fmt.Errorf("HTTP request returned status %d", resp.StatusCode)
	}

	// Parse response
	var serverInfo types.SystemInfoResponse
	logging.Logf(types.LogDebug, "Attempting to decode JSON response body")
	err = json.NewDecoder(resp.Body).Decode(&serverInfo)
	if err != nil {
		logging.Logf(types.LogDebug, "JSON decode error type: %T", err)
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	logging.Logf(types.LogInfo, "Successfully retrieved server info from API (Server: %s, ID: %s)", serverInfo.ServerName, serverInfo.Id)
	logging.Logf(types.LogDebug, "Decoded server info - ServerName: '%s', Id: '%s'", serverInfo.ServerName, serverInfo.Id)
	return &serverInfo, nil
}

// IsHostname checks if a URL contains a hostname rather than an IP address
func IsHostname(urlStr string) bool {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	host, _, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		host = parsedURL.Host
	}

	// Check if it's an IP address
	if net.ParseIP(host) != nil {
		return false
	}

	return true
}

// ResolveHostnameToIP resolves a hostname URL to its IP address equivalent
func ResolveHostnameToIP(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	host, port, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		host = parsedURL.Host
		port = ""
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}

	// Find the first non-loopback IPv4 address
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			// Skip loopback addresses (127.x.x.x)
			if !ip.IsLoopback() {
				newHost := ipv4.String()
				if port != "" {
					newHost = net.JoinHostPort(newHost, port)
				}
				parsedURL.Host = newHost
				return parsedURL.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no IPv4 address found for hostname %s", host)
}
