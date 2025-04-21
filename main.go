package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// JellyfinDiscoveryResponse represents the response format for Jellyfin discovery
type JellyfinDiscoveryResponse struct {
	Address         string      `json:"Address"`
	Id              string      `json:"Id"`
	Name            string      `json:"Name"`
	EndpointAddress interface{} `json:"EndpointAddress"`
}

// SystemInfoResponse represents the response from the Jellyfin System/Info endpoint
type SystemInfoResponse struct {
	Id         string `json:"Id"`
	ServerName string `json:"ServerName"`
}

// ServerInfoCache holds the cached server information and its timestamp
type ServerInfoCache struct {
	Info      *SystemInfoResponse
	Timestamp time.Time
	mutex     sync.RWMutex
}

// NewServerInfoCache creates a new cache instance
func NewServerInfoCache() *ServerInfoCache {
	return &ServerInfoCache{
		Info:      nil,
		Timestamp: time.Time{},
		mutex:     sync.RWMutex{},
	}
}

// Get returns the cached info if valid, or nil if expired or not set
func (c *ServerInfoCache) Get() *SystemInfoResponse {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.Info != nil && time.Since(c.Timestamp) < 24*time.Hour {
		return c.Info
	}
	return nil
}

// Set updates the cache with new server info
func (c *ServerInfoCache) Set(info *SystemInfoResponse) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.Info = info
	c.Timestamp = time.Now()
}

func main() {
	// Get server URL from environment variable or use default
	serverURL := os.Getenv("JELLYFIN_SERVER_URL")
	if serverURL == "" {
		log.Println("JELLYFIN_SERVER_URL environment variable not set, using default http://localhost:8096")
		serverURL = "http://localhost:8096"
	}

	// Remove trailing slash if present
	serverURL = strings.TrimSuffix(serverURL, "/")

	log.Printf("Starting Jellyfin Discovery Proxy for server: %s", serverURL)

	// Create UDP address for listening
	addr, err := net.ResolveUDPAddr("udp", ":7359")
	if err != nil {
		log.Fatalf("Error resolving UDP address: %v", err)
	}

	// Create UDP listener
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Error listening on UDP port 7359: %v", err)
	}
	defer conn.Close()

	log.Println("Listening for Jellyfin discovery requests on UDP port 7359")
	log.Println("Server info will be cached for 24 hours")

	// Create a buffer for incoming messages
	buffer := make([]byte, 1024)

	// Initialize cache
	cache := NewServerInfoCache()

	// Fetch server info once at startup
	serverInfo, err := fetchServerInfo(serverURL)
	if err != nil {
		log.Printf("Warning: Could not fetch server info at startup: %v", err)
		log.Println("Will try again when discovery requests are received")
	} else {
		log.Printf("Successfully fetched server info at startup. Server ID: %s, Name: %s",
			serverInfo.Id, serverInfo.ServerName)
		cache.Set(serverInfo)
	}

	// Listen for incoming requests
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Error reading UDP message: %v", err)
			continue
		}

		message := string(buffer[:n])
		log.Printf("Received %d bytes from %s: %s", n, addr.String(), message)

		// Check if this is a Jellyfin discovery request
		if message == "Who is JellyfinServer?" {
			go handleDiscoveryRequest(conn, addr, serverURL, cache)
		}
	}
}

func handleDiscoveryRequest(conn *net.UDPConn, addr *net.UDPAddr, serverURL string, cache *ServerInfoCache) {
	log.Printf("Handling discovery request from %s", addr.String())

	// Try to get info from cache first
	serverInfo := cache.Get()

	// If cache is empty or expired, fetch fresh info
	if serverInfo == nil {
		log.Println("Cache expired or empty, fetching fresh server info")
		var err error
		serverInfo, err = fetchServerInfo(serverURL)
		if err != nil {
			log.Printf("Error fetching server info: %v", err)
			log.Println("Not responding to discovery request as server is unreachable")
			return // Don't respond if server is unreachable
		}

		// Update cache with new info
		cache.Set(serverInfo)
	}

	// Create response
	response := JellyfinDiscoveryResponse{
		Address:         serverURL,
		Id:              serverInfo.Id,
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}

	// Serialize to JSON
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling JSON response: %v", err)
		return
	}

	// Send response
	_, err = conn.WriteToUDP(jsonResponse, addr)
	if err != nil {
		log.Printf("Error sending response: %v", err)
		return
	}

	log.Printf("Sent discovery response to %s: %s", addr.String(), string(jsonResponse))
}

func fetchServerInfo(serverURL string) (*SystemInfoResponse, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Call the Jellyfin system info endpoint
	infoURL := fmt.Sprintf("%s/System/Info/Public", serverURL)
	log.Printf("Fetching server info from: %s", infoURL)

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
