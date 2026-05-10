package discovery

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/hooks"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/logging"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/server"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// ListenLoop listens for IPv4 discovery requests on a single UDP socket and
// emits responses for proxyURL plus, when configured, proxyURLv6 so
// dual-stack clients can pick whichever endpoint they prefer.
func ListenLoop(ctx context.Context, conn *net.UDPConn,
	serverURL, proxyURL, proxyURLv6 string,
	cache *types.ServerInfoCache,
	blacklist *types.IPBlacklist, stats *types.RequestStats, hookConfig *hooks.HookConfig) {
	buffer := make([]byte, 1024)
	logging.Logf(types.LogDebug, "Listener started for %s with buffer size: %d bytes", conn.LocalAddr(), len(buffer))

	for {
		select {
		case <-ctx.Done():
			logging.Logf(types.LogDebug, "Context cancelled, stopping listener for %s", conn.LocalAddr())
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		logging.Logf(types.LogDebug, "Waiting for UDP packet on %s", conn.LocalAddr())
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
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

		if strings.EqualFold(message, "Who is JellyfinServer?") {
			logging.Logf(types.LogDebug, "Valid Jellyfin discovery request detected, spawning handler goroutine")
			go HandleRequest(conn, addr, serverURL, proxyURL, proxyURLv6, cache, blacklist, stats, hookConfig)
		} else {
			logging.Logf(types.LogWarn, "Ignoring unrecognized message from %s: %s", addr.String(), message)
			logging.Logf(types.LogDebug, "Expected 'Who is JellyfinServer?' but got '%s'", message)
		}
	}
}

// HandleRequest processes a discovery request, fetching server info if the
// cache is cold, then emitting the primary response and (when configured) a
// second response carrying proxyURLv6.
func HandleRequest(conn *net.UDPConn, addr *net.UDPAddr,
	serverURL, proxyURL, proxyURLv6 string,
	cache *types.ServerInfoCache,
	blacklist *types.IPBlacklist, stats *types.RequestStats, hookConfig *hooks.HookConfig) {
	logging.Logf(types.LogInfo, "Processing discovery request from %s", addr.String())
	logging.Logf(types.LogDebug, "Handler goroutine started for request from %s", addr.String())

	clientIP := addr.IP.String()
	if blacklist.IsBlocked(clientIP) {
		logging.Logf(types.LogWarn, "Ignoring request from blacklisted IP: %s", clientIP)
		return
	}

	hookConfig.ExecuteOnReceive(hooks.OnReceivePayload{
		Timestamp:   time.Now(),
		ClientIP:    clientIP,
		ClientPort:  addr.Port,
		Message:     "Who is JellyfinServer?",
		LocalSocket: conn.LocalAddr().String(),
	})

	stats.RecordRequest(clientIP)

	logging.Logln(types.LogDebug, "Checking cache for server info")
	serverInfo := cache.Get()

	if serverInfo == nil {
		logging.Logln(types.LogInfo, "Cache expired or empty, fetching fresh server info from Jellyfin")
		logging.Logf(types.LogDebug, "Cache miss - last cached at: %v, cache duration: %v", cache.Timestamp, cache.Duration)

		var err error
		serverInfo, err = server.FetchInfo(serverURL)
		if err != nil {
			logging.Logf(types.LogError, "Failed to fetch server info: %v", err)
			logging.Logf(types.LogDebug, "Fetch error type: %T", err)
			logging.Logf(types.LogWarn, "Not responding to discovery request from %s - server is unreachable", addr.String())
			return
		}

		cache.Set(serverInfo)
		logging.Logln(types.LogInfo, "Successfully updated cache with fresh server info")
		logging.Logf(types.LogDebug, "Cache updated at: %v", cache.Timestamp)
	} else {
		logging.Logln(types.LogInfo, "Using cached server info for response")
		logging.Logf(types.LogDebug, "Cache hit - age: %v, cached at: %v", time.Since(cache.Timestamp), cache.Timestamp)
	}

	sendForURL(conn, addr, proxyURL, serverInfo, hookConfig, "primary")

	// Only emit a second response when an IPv6-specific URL was configured;
	// otherwise it would just duplicate the primary payload.
	if proxyURLv6 != "" && proxyURLv6 != proxyURL {
		sendForURL(conn, addr, proxyURLv6, serverInfo, hookConfig, "IPv6")
	}

	logging.Logf(types.LogDebug, "Handler goroutine completed for %s", addr.String())
}

// sendForURL dispatches the discovery response for a single advertised URL,
// expanding hostnames to "hostname + resolved IP" pairs for non-Avahi device
// compatibility (matches the behavior the proxy has had since hostnames were
// first supported).
func sendForURL(conn *net.UDPConn, addr *net.UDPAddr, advertisedURL string, serverInfo *types.SystemInfoResponse, hookConfig *hooks.HookConfig, label string) {
	if advertisedURL == "" {
		return
	}

	if server.IsHostname(advertisedURL) {
		logging.Logf(types.LogInfo, "Sending dual %s responses (hostname + IP) for non-Avahi device compatibility", label)
		logging.Logf(types.LogDebug, "%s dual response mode enabled for hostname: %s", label, advertisedURL)

		SendResponse(conn, addr, advertisedURL, serverInfo, hookConfig)

		logging.Logf(types.LogDebug, "Attempting to resolve %s hostname %s to IP", label, advertisedURL)
		ipURL, err := server.ResolveHostnameToIP(advertisedURL)
		if err != nil {
			logging.Logf(types.LogWarn, "Could not resolve %s hostname %s to IP: %v", label, advertisedURL, err)
			logging.Logf(types.LogDebug, "%s DNS resolution error type: %T", label, err)
			return
		}
		logging.Logf(types.LogInfo, "Resolved %s %s to %s, sending second response", label, advertisedURL, ipURL)
		logging.Logf(types.LogDebug, "%s hostname resolved successfully to: %s", label, ipURL)
		SendResponse(conn, addr, ipURL, serverInfo, hookConfig)
		return
	}

	logging.Logf(types.LogDebug, "%s single response mode - sending one discovery response", label)
	SendResponse(conn, addr, advertisedURL, serverInfo, hookConfig)
}

// SendResponse sends a single discovery response to the client.
func SendResponse(conn *net.UDPConn, addr *net.UDPAddr, addressURL string, serverInfo *types.SystemInfoResponse, hookConfig *hooks.HookConfig) {
	logging.Logf(types.LogDebug, "Constructing discovery response for %s", addr.String())

	response := types.JellyfinDiscoveryResponse{
		Address:         addressURL,
		Id:              serverInfo.Id,
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}
	logging.Logf(types.LogDebug, "Response struct - Address: %s, Id: %s, Name: %s", response.Address, response.Id, response.Name)

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		logging.Logf(types.LogError, "Error marshaling JSON response: %v", err)
		logging.Logf(types.LogDebug, "JSON marshal error type: %T", err)
		return
	}
	logging.Logf(types.LogDebug, "JSON response length: %d bytes", len(jsonResponse))
	logging.Logf(types.LogDebug, "JSON response content: %s", string(jsonResponse))

	hookConfig.ExecuteOnSend(hooks.OnSendPayload{
		Timestamp:     time.Now(),
		ClientIP:      addr.IP.String(),
		ClientPort:    addr.Port,
		ServerID:      serverInfo.Id,
		ServerName:    serverInfo.ServerName,
		AddressURL:    addressURL,
		ResponseBytes: len(jsonResponse),
	})

	bytesWritten, err := conn.WriteToUDP(jsonResponse, addr)
	if err != nil {
		logging.Logf(types.LogError, "Error sending response to %s: %v", addr.String(), err)
		logging.Logf(types.LogDebug, "UDP write error type: %T", err)
		return
	}

	logging.Logf(types.LogInfo, "Sent discovery response to %s | Server: %s | Address: %s", addr.String(), serverInfo.ServerName, addressURL)
	logging.Logf(types.LogDebug, "Successfully sent %d bytes to %s", bytesWritten, addr.String())
}
