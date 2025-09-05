package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// logf - Custom logging function with level prefix
// Levels: "INF" (info), "WRN" (warning), "ERR" (error)
func logf(level string, format string, v ...interface{}) {
	// Format the message with the level prefix
	prefix := fmt.Sprintf("%s: ", level)
	formattedMsg := fmt.Sprintf(format, v...)
	log.Print(prefix + formattedMsg)
}

// log - Custom logging function with level prefix for simple messages
// Levels: "INF" (info), "WRN" (warning), "ERR" (error)
func logln(level string, message string) {
	// Format the message with the level prefix
	prefix := fmt.Sprintf("%s: ", level)
	log.Println(prefix + message)
}

// JellyfinDiscoveryResponse
// Type - Jellyfin Discovery Response JSON Format
type JellyfinDiscoveryResponse struct {
	Address         string      `json:"Address"`
	Id              string      `json:"Id"`
	Name            string      `json:"Name"`
	EndpointAddress interface{} `json:"EndpointAddress"`
}

// SystemInfoResponse
// Type - Jellyfin System/Info Endpoint Response Relevant Information
type SystemInfoResponse struct {
	Id         string `json:"Id"`
	ServerName string `json:"ServerName"`
}

// ServerInfoCache
// Type - Cached Server Information and its Last Timestamp
type ServerInfoCache struct {
	Info      *SystemInfoResponse
	Timestamp time.Time
	Duration  time.Duration
	mutex     sync.RWMutex
}

// NewServerInfoCache()
// Func - Creates a New Empty ServerInfoCache Instance with Specified Cache Duration
func NewServerInfoCache(cacheDuration time.Duration) *ServerInfoCache {
	return &ServerInfoCache{
		Info:      nil,
		Timestamp: time.Time{},
		Duration:  cacheDuration,
		mutex:     sync.RWMutex{},
	}
}

// Get()
// Func - Returns the Cached ServerInfo or nil if Cache is Empty or Expired
func (c *ServerInfoCache) Get() *SystemInfoResponse {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// If Duration is 0, cache never expires (until restart)
	if c.Info != nil && (c.Duration == 0 || time.Since(c.Timestamp) < c.Duration) {
		return c.Info
	}
	return nil
}

// Set()
// Func - Updates the Cache with New Server Information and Current Timestamp
func (c *ServerInfoCache) Set(info *SystemInfoResponse) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.Info = info
	c.Timestamp = time.Now()
}

// GetCacheDuration()
// Func - Parses CACHE_DURATION Environment Variable and Returns Appropriate Duration
func GetCacheDuration() time.Duration {
	cacheDurationStr := os.Getenv("CACHE_DURATION")
	if cacheDurationStr == "" {
		logln("INF", "CACHE_DURATION environment variable not set, using default 24 hours")
		return 24 * time.Hour
	}

	// If explicitly set to 0, cache until restart
	if cacheDurationStr == "0" {
		logln("INF", "CACHE_DURATION set to 0, caching until restart")
		return 0
	}

	// Parse the hours value
	hours, err := strconv.Atoi(cacheDurationStr)
	if err != nil {
		logf("WRN", "Invalid CACHE_DURATION value: %s, using default 24 hours", cacheDurationStr)
		return 24 * time.Hour
	}

	logf("INF", "CACHE_DURATION set to %d hours", hours)
	return time.Duration(hours) * time.Hour
}

// MARK: isHostname()
// Func - Checks if a URL contains a hostname rather than an IP address
func isHostname(urlStr string) bool {
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

// MARK: resolveHostnameToIP()
// Func - Resolves a hostname URL to its IP address equivalent
func resolveHostnameToIP(urlStr string) (string, error) {
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

	// Find the first IPv4 address
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			newHost := ipv4.String()
			if port != "" {
				newHost = net.JoinHostPort(newHost, port)
			}
			parsedURL.Host = newHost
			return parsedURL.String(), nil
		}
	}

	return "", fmt.Errorf("no IPv4 address found for hostname %s", host)
}

// main()
// Func - Main Function That Initializes and Runs the Jellyfin Discovery Proxy
func main() {
	// Get server URL from environment variable or use default
	serverURL := os.Getenv("JELLYFIN_SERVER_URL")
	if serverURL == "" {
		logln("INF", "JELLYFIN_SERVER_URL environment variable not set, using default http://localhost:8096")
		serverURL = "http://localhost:8096"
	}

	// Get proxy URL from environment variable
	proxyURL := os.Getenv("PROXY_URL")
	if proxyURL == "" {
		logln("INF", "PROXY_URL environment variable not set, will use JELLYFIN_SERVER_URL for Address field")
	} else {
		logf("INF", "PROXY_URL set to %s, will use for Address field in responses", proxyURL)
		if isHostname(proxyURL) {
			logln("INF", "PROXY_URL is a hostname, will broadcast both hostname and IP responses for Roku compatibility")
		}
	}

	// Remove trailing slash if present from URLs
	serverURL = strings.TrimSuffix(serverURL, "/")
	if proxyURL != "" {
		proxyURL = strings.TrimSuffix(proxyURL, "/")
	}

	logf("INF", "Starting Jellyfin Discovery Proxy for server: %s", serverURL)

	// Determine cache duration
	cacheDuration := GetCacheDuration()

	// Create UDP address for listening
	addr, err := net.ResolveUDPAddr("udp", ":7359")
	if err != nil {
		logf("ERR", "Error resolving UDP address: %v", err)
		os.Exit(1)
	}

	// Create UDP listener
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		logf("ERR", "Error listening on UDP port 7359: %v", err)
		os.Exit(1)
	}
	defer conn.Close()

	logln("INF", "Listening for Jellyfin discovery requests on UDP port 7359")
	if cacheDuration == 0 {
		logln("INF", "Server info will be cached until restart")
	} else {
		logf("INF", "Server info will be cached for %v", cacheDuration)
	}

	// Create a buffer for incoming messages
	buffer := make([]byte, 1024)

	// Initialize cache with determined duration
	cache := NewServerInfoCache(cacheDuration)

	// Fetch server info once at startup
	serverInfo, err := fetchServerInfo(serverURL)
	if err != nil {
		logf("WRN", "Could not fetch server info at startup: %v", err)
		logln("WRN", "Will try again when discovery requests are received")
	} else {
		logf("INF", "Successfully fetched server info at startup. Server ID: %s, Name: %s",
			serverInfo.Id, serverInfo.ServerName)
		cache.Set(serverInfo)
	}

	// Listen for incoming requests
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			logf("ERR", "Error reading UDP message: %v", err)
			continue
		}

		message := string(buffer[:n])
		logf("INF", "Received %d bytes from %s: %s", n, addr.String(), message)

		// Check if this is a Jellyfin discovery request
		if strings.EqualFold(message, "Who is JellyfinServer?") {
			go handleDiscoveryRequest(conn, addr, serverURL, proxyURL, cache)
		}
	}
}

// handleDiscoveryRequest()
// Func - Processes Jellyfin Discovery Requests and Sends Appropriate Responses
func handleDiscoveryRequest(conn *net.UDPConn, addr *net.UDPAddr, serverURL string, proxyURL string, cache *ServerInfoCache) {
	logf("INF", "Handling discovery request from %s", addr.String())

	// Try to get info from cache first
	serverInfo := cache.Get()

	// If cache is empty or expired, fetch fresh info
	if serverInfo == nil {
		logln("INF", "Cache expired or empty, fetching fresh server info")
		var err error
		serverInfo, err = fetchServerInfo(serverURL)
		if err != nil {
			logf("ERR", "Error fetching server info: %v", err)
			logln("WRN", "Not responding to discovery request as server is unreachable")
			return // Don't respond if server is unreachable
		}

		// Update cache with new info
		cache.Set(serverInfo)
	}

	// Determine which URL to use for the Address field
	addressURL := ""

	if proxyURL != "" {
		addressURL = proxyURL
	} else {
		addressURL = serverURL
	}

	// If proxy URL is a hostname, send two responses: hostname and IP
	if proxyURL != "" && isHostname(proxyURL) {
		// Send first response with hostname
		sendDiscoveryResponse(conn, addr, addressURL, serverInfo)

		// Try to resolve hostname to IP and send second response
		ipURL, err := resolveHostnameToIP(proxyURL)
		if err != nil {
			logf("WRN", "Could not resolve hostname %s to IP: %v", proxyURL, err)
		} else {
			logf("INF", "Resolved %s to %s, sending second response", proxyURL, ipURL)
			sendDiscoveryResponse(conn, addr, ipURL, serverInfo)
		}
	} else {
		// Send single response
		sendDiscoveryResponse(conn, addr, addressURL, serverInfo)
	}
}

// MARK: sendDiscoveryResponse()
// Func - Sends a single discovery response with the given address URL
func sendDiscoveryResponse(conn *net.UDPConn, addr *net.UDPAddr, addressURL string, serverInfo *SystemInfoResponse) {
	// Create response
	response := JellyfinDiscoveryResponse{
		Address:         addressURL,
		Id:              serverInfo.Id,
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}

	// Serialize to JSON
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		logf("ERR", "Error marshaling JSON response: %v", err)
		return
	}

	// Send response
	_, err = conn.WriteToUDP(jsonResponse, addr)
	if err != nil {
		logf("ERR", "Error sending response: %v", err)
		return
	}

	logf("INF", "Sent discovery response to %s: %s", addr.String(), string(jsonResponse))
}

// fetchServerInfo()
// Func - Retrieves Server Information from Jellyfin System/Info Endpoint
func fetchServerInfo(serverURL string) (*SystemInfoResponse, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Call the Jellyfin system info endpoint
	infoURL := fmt.Sprintf("%s/System/Info/Public", serverURL)
	logf("INF", "Fetching server info from: %s", infoURL)

	resp, err := client.Get(infoURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request returned non-OK status: %d", resp.StatusCode)
	}

	// Parse response
	var serverInfo SystemInfoResponse
	err = json.NewDecoder(resp.Body).Decode(&serverInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	return &serverInfo, nil
}
