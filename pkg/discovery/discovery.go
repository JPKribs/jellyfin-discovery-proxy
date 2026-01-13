package discovery

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/logging"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/server"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// ListenLoop listens for discovery requests on a single UDP socket
func ListenLoop(ctx context.Context, conn *net.UDPConn, serverURL, proxyURL string, cache *types.ServerInfoCache, blacklist *types.IPBlacklist, stats *types.RequestStats) {
	buffer := make([]byte, 1024)
	logging.Logf(types.LogDebug, "Listener started for %s with buffer size: %d bytes", conn.LocalAddr(), len(buffer))

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			logging.Logf(types.LogDebug, "Context cancelled, stopping listener for %s", conn.LocalAddr())
			return
		default:
		}

		// Set a read deadline so we can check context periodically
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		logging.Logf(types.LogDebug, "Waiting for UDP packet on %s", conn.LocalAddr())
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			// Check if it's a timeout error (normal during shutdown check)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Check if connection is closed (during shutdown)
			if ctx.Err() != nil {
				logging.Logf(types.LogDebug, "Connection closed during shutdown: %s", conn.LocalAddr())
				return
			}
			logging.Logf(types.LogError, "Error reading UDP message: %v", err)
			logging.Logf(types.LogDebug, "UDP read error type: %T, connection: %s", err, conn.LocalAddr())
			continue
		}

		message := string(buffer[:n])
		logging.Logf(types.LogInfo, "Received discovery request from %s (%d bytes): %s", addr.String(), n, message)
		logging.Logf(types.LogDebug, "Message hex dump: % X", buffer[:n])
		logging.Logf(types.LogDebug, "Remote address details - IP: %s, Port: %d, Zone: %s", addr.IP, addr.Port, addr.Zone)

		// Check if this is a Jellyfin discovery request
		if strings.EqualFold(message, "Who is JellyfinServer?") {
			logging.Logf(types.LogDebug, "Valid Jellyfin discovery request detected, spawning handler goroutine")
			go HandleRequest(conn, addr, serverURL, proxyURL, cache, blacklist, stats)
		} else {
			logging.Logf(types.LogWarn, "Ignoring unrecognized message from %s: %s", addr.String(), message)
			logging.Logf(types.LogDebug, "Expected 'Who is JellyfinServer?' but got '%s'", message)
		}
	}
}

// HandleRequest processes Jellyfin Discovery Requests and sends appropriate responses
func HandleRequest(conn *net.UDPConn, addr *net.UDPAddr, serverURL string, proxyURL string, cache *types.ServerInfoCache, blacklist *types.IPBlacklist, stats *types.RequestStats) {
	logging.Logf(types.LogInfo, "Processing discovery request from %s", addr.String())
	logging.Logf(types.LogDebug, "Handler goroutine started for request from %s", addr.String())

	// Check if IP is blacklisted
	clientIP := addr.IP.String()
	if blacklist.IsBlocked(clientIP) {
		logging.Logf(types.LogWarn, "Ignoring request from blacklisted IP: %s", clientIP)
		return
	}

	// Record request statistics
	stats.RecordRequest(clientIP)

	// Try to get info from cache first
	logging.Logf(types.LogDebug, "Checking cache for server info")
	serverInfo := cache.Get()

	// If cache is empty or expired, fetch fresh info
	if serverInfo == nil {
		logging.Logln(types.LogInfo, "Cache expired or empty, fetching fresh server info from Jellyfin")
		logging.Logf(types.LogDebug, "Cache miss - last cached at: %v, cache duration: %v", cache.Timestamp, cache.Duration)

		var err error
		serverInfo, err = server.FetchInfo(serverURL)
		if err != nil {
			logging.Logf(types.LogError, "Failed to fetch server info: %v", err)
			logging.Logf(types.LogDebug, "Fetch error type: %T", err)
			logging.Logf(types.LogWarn, "Not responding to discovery request from %s - server is unreachable", addr.String())
			return // Don't respond if server is unreachable
		}

		// Update cache with new info
		cache.Set(serverInfo)
		logging.Logln(types.LogInfo, "Successfully updated cache with fresh server info")
		logging.Logf(types.LogDebug, "Cache updated at: %v", cache.Timestamp)
	} else {
		logging.Logln(types.LogInfo, "Using cached server info for response")
		cacheAge := time.Since(cache.Timestamp)
		logging.Logf(types.LogDebug, "Cache hit - age: %v, cached at: %v", cacheAge, cache.Timestamp)
	}

	// Determine which URL to use for the Address field
	addressURL := ""

	if proxyURL != "" {
		addressURL = proxyURL
		logging.Logf(types.LogDebug, "Using PROXY_URL for address: %s", addressURL)
	} else {
		addressURL = serverURL
		logging.Logf(types.LogDebug, "Using JELLYFIN_SERVER_URL for address: %s", addressURL)
	}

	// If proxy URL is a hostname, send two responses: hostname and IP
	if proxyURL != "" && server.IsHostname(proxyURL) {
		logging.Logln(types.LogInfo, "Sending dual responses (hostname + IP) for non-Avahi device compatibility")
		logging.Logf(types.LogDebug, "Dual response mode enabled for hostname: %s", proxyURL)

		// Send first response with hostname
		SendResponse(conn, addr, addressURL, serverInfo)

		// Try to resolve hostname to IP and send second response
		logging.Logf(types.LogDebug, "Attempting to resolve hostname %s to IP", proxyURL)
		ipURL, err := server.ResolveHostnameToIP(proxyURL)
		if err != nil {
			logging.Logf(types.LogWarn, "Could not resolve hostname %s to IP: %v", proxyURL, err)
			logging.Logf(types.LogDebug, "DNS resolution error type: %T", err)
		} else {
			logging.Logf(types.LogInfo, "Resolved %s to %s, sending second response", proxyURL, ipURL)
			logging.Logf(types.LogDebug, "Hostname resolved successfully to: %s", ipURL)
			SendResponse(conn, addr, ipURL, serverInfo)
		}
	} else {
		// Send single response
		logging.Logf(types.LogDebug, "Single response mode - sending one discovery response")
		SendResponse(conn, addr, addressURL, serverInfo)
	}

	logging.Logf(types.LogDebug, "Handler goroutine completed for %s", addr.String())
}

// SendResponse sends a single discovery response with the given address URL
func SendResponse(conn *net.UDPConn, addr *net.UDPAddr, addressURL string, serverInfo *types.SystemInfoResponse) {
	logging.Logf(types.LogDebug, "Constructing discovery response for %s", addr.String())

	// Create response
	response := types.JellyfinDiscoveryResponse{
		Address:         addressURL,
		Id:              serverInfo.Id,
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}
	logging.Logf(types.LogDebug, "Response struct - Address: %s, Id: %s, Name: %s", response.Address, response.Id, response.Name)

	// Serialize to JSON
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		logging.Logf(types.LogError, "Error marshaling JSON response: %v", err)
		logging.Logf(types.LogDebug, "JSON marshal error type: %T", err)
		return
	}
	logging.Logf(types.LogDebug, "JSON response length: %d bytes", len(jsonResponse))
	logging.Logf(types.LogDebug, "JSON response content: %s", string(jsonResponse))

	// Send response
	bytesWritten, err := conn.WriteToUDP(jsonResponse, addr)
	if err != nil {
		logging.Logf(types.LogError, "Error sending response to %s: %v", addr.String(), err)
		logging.Logf(types.LogDebug, "UDP write error type: %T", err)
		return
	}

	logging.Logf(types.LogInfo, "Sent discovery response to %s | Server: %s | Address: %s", addr.String(), serverInfo.ServerName, addressURL)
	logging.Logf(types.LogDebug, "Successfully sent %d bytes to %s", bytesWritten, addr.String())
}
