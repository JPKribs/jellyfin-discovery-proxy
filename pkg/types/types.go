package types

import (
	"net"
	"sync"
	"time"
)

// Version information
const Version = "1.3.3"

// Log represents the severity of a log message
type Log int

const (
	// LogDebug for detailed debugging information
	LogDebug Log = iota
	// LogInfo for general informational messages
	LogInfo
	// LogWarn for warning messages
	LogWarn
	// LogError for error messages
	LogError
)

// String returns the string representation of the log level
func (l Log) String() string {
	switch l {
	case LogDebug:
		return "DEB"
	case LogInfo:
		return "INF"
	case LogWarn:
		return "WRN"
	case LogError:
		return "ERR"
	default:
		return "???"
	}
}

// JellyfinDiscoveryResponse represents the Jellyfin Discovery Response JSON Format
type JellyfinDiscoveryResponse struct {
	Address         string      `json:"Address"`
	Id              string      `json:"Id"`
	Name            string      `json:"Name"`
	EndpointAddress interface{} `json:"EndpointAddress"`
}

// SystemInfoResponse represents the Jellyfin System/Info Endpoint Response Relevant Information
type SystemInfoResponse struct {
	Id         string `json:"Id"`
	ServerName string `json:"ServerName"`
}

// ServerInfoCache holds cached server information and its last timestamp
type ServerInfoCache struct {
	Info      *SystemInfoResponse
	Timestamp time.Time
	Duration  time.Duration
	Mutex     sync.RWMutex
}

// DashboardData holds data for the dashboard template
type DashboardData struct {
	Version            string
	ServerURLv4        string
	ServerURLv6        string
	ProxyURLv4         string
	ProxyURLv6         string
	LastRequestTime    string
	LastRequestIP      string
	TotalRequests      int64
	CachedServerIDv4   string
	CachedServerNamev4 string
	CacheAgev4         string
	CachedServerIDv6   string
	CachedServerNamev6 string
	CacheAgev6         string
	BlacklistedIPs     int
	Logs               []string
	Uptime             string
}

// LogBuffer holds recent log messages in memory
type LogBuffer struct {
	Messages []string
	MaxSize  int
	Mutex    sync.RWMutex
}

// RequestStats tracks statistics about discovery requests
type RequestStats struct {
	LastRequestTime time.Time
	LastRequestIP   string
	TotalRequests   int64
	Mutex           sync.RWMutex
}

// IPBlacklist manages blocked IP addresses and subnets
type IPBlacklist struct {
	IPs     map[string]bool
	Subnets []*net.IPNet
	Mutex   sync.RWMutex
}

// Config holds all configuration for the proxy
type Config struct {
	ServerURLv4      string
	ServerURLv6      string
	ProxyURLv4       string
	ProxyURLv6       string
	NetworkInterface string
	BindIP           string
}

// ServerInfoCache methods

// Get returns the cached ServerInfo or nil if cache is empty or expired
func (c *ServerInfoCache) Get() *SystemInfoResponse {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	// If Duration is 0, cache never expires (until restart)
	if c.Info != nil && (c.Duration == 0 || time.Since(c.Timestamp) < c.Duration) {
		return c.Info
	}
	return nil
}

// Set updates the cache with new server information and current timestamp
func (c *ServerInfoCache) Set(info *SystemInfoResponse) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	c.Info = info
	c.Timestamp = time.Now()
}

// LogBuffer methods

// Add adds a log message to the buffer
func (lb *LogBuffer) Add(message string) {
	lb.Mutex.Lock()
	defer lb.Mutex.Unlock()

	lb.Messages = append(lb.Messages, message)
	if len(lb.Messages) > lb.MaxSize {
		lb.Messages = lb.Messages[1:]
	}
}

// GetAll returns all log messages
func (lb *LogBuffer) GetAll() []string {
	lb.Mutex.RLock()
	defer lb.Mutex.RUnlock()

	result := make([]string, len(lb.Messages))
	copy(result, lb.Messages)
	return result
}

// RequestStats methods

// RecordRequest records a discovery request
func (rs *RequestStats) RecordRequest(ip string) {
	rs.Mutex.Lock()
	defer rs.Mutex.Unlock()

	rs.LastRequestTime = time.Now()
	rs.LastRequestIP = ip
	rs.TotalRequests++
}

// GetStats returns current stats
func (rs *RequestStats) GetStats() (time.Time, string, int64) {
	rs.Mutex.RLock()
	defer rs.Mutex.RUnlock()

	return rs.LastRequestTime, rs.LastRequestIP, rs.TotalRequests
}

// IPBlacklist methods

// IsBlocked checks if an IP is blacklisted (either as individual IP or within a subnet)
func (bl *IPBlacklist) IsBlocked(ipStr string) bool {
	bl.Mutex.RLock()
	defer bl.Mutex.RUnlock()

	// Check individual IPs first (faster)
	if bl.IPs[ipStr] {
		return true
	}

	// Parse the IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Check if IP is in any blacklisted subnet
	for _, subnet := range bl.Subnets {
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// Count returns the total number of blacklisted IPs and subnets
func (bl *IPBlacklist) Count() int {
	bl.Mutex.RLock()
	defer bl.Mutex.RUnlock()

	return len(bl.IPs) + len(bl.Subnets)
}
