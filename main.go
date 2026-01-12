package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Version information
const Version = "1.3.0"

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

// Global log level configuration
var currentLog Log = LogInfo

// LogBuffer holds recent log messages in memory
type LogBuffer struct {
	messages []string
	maxSize  int
	mutex    sync.RWMutex
}

// NewLogBuffer creates a new log buffer with specified max size
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		messages: make([]string, 0, maxSize),
		maxSize:  maxSize,
	}
}

// Add adds a log message to the buffer
func (lb *LogBuffer) Add(message string) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	lb.messages = append(lb.messages, message)
	if len(lb.messages) > lb.maxSize {
		lb.messages = lb.messages[1:]
	}
}

// GetAll returns all log messages
func (lb *LogBuffer) GetAll() []string {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	result := make([]string, len(lb.messages))
	copy(result, lb.messages)
	return result
}

// Global log buffer
var logBuffer *LogBuffer

// RequestStats tracks statistics about discovery requests
type RequestStats struct {
	LastRequestTime time.Time
	LastRequestIP   string
	TotalRequests   int64
	mutex           sync.RWMutex
}

// Global request stats
var requestStats = &RequestStats{}

// RecordRequest records a discovery request
func (rs *RequestStats) RecordRequest(ip string) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	rs.LastRequestTime = time.Now()
	rs.LastRequestIP = ip
	rs.TotalRequests++
}

// GetStats returns current stats
func (rs *RequestStats) GetStats() (time.Time, string, int64) {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()

	return rs.LastRequestTime, rs.LastRequestIP, rs.TotalRequests
}

// IPBlacklist manages blocked IP addresses and subnets
type IPBlacklist struct {
	ips     map[string]bool
	subnets []*net.IPNet
	mutex   sync.RWMutex
}

// NewIPBlacklist creates a new IP blacklist from comma-separated string
// Supports both individual IPs (192.168.1.100) and CIDR notation (192.168.1.0/24)
func NewIPBlacklist(blacklistStr string) *IPBlacklist {
	bl := &IPBlacklist{
		ips:     make(map[string]bool),
		subnets: make([]*net.IPNet, 0),
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
				logf(LogWarn, "Invalid CIDR notation in blacklist: %s, skipping", entry)
				continue
			}
			bl.subnets = append(bl.subnets, ipnet)
			logf(LogDebug, "Added subnet to blacklist: %s", entry)
		} else {
			// Individual IP address
			parsedIP := net.ParseIP(entry)
			if parsedIP == nil {
				logf(LogWarn, "Invalid IP address in blacklist: %s, skipping", entry)
				continue
			}
			bl.ips[entry] = true
			logf(LogDebug, "Added IP to blacklist: %s", entry)
		}
	}

	return bl
}

// IsBlocked checks if an IP is blacklisted (either as individual IP or within a subnet)
func (bl *IPBlacklist) IsBlocked(ipStr string) bool {
	bl.mutex.RLock()
	defer bl.mutex.RUnlock()

	// Check individual IPs first (faster)
	if bl.ips[ipStr] {
		return true
	}

	// Parse the IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Check if IP is in any blacklisted subnet
	for _, subnet := range bl.subnets {
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// Count returns the total number of blacklisted IPs and subnets
func (bl *IPBlacklist) Count() int {
	bl.mutex.RLock()
	defer bl.mutex.RUnlock()

	return len(bl.ips) + len(bl.subnets)
}

// Global IP blacklist
var ipBlacklist *IPBlacklist

// setLog parses and sets the global log level from a string
func setLog(level string) {
	switch strings.ToLower(level) {
	case "debug":
		currentLog = LogDebug
	case "info":
		currentLog = LogInfo
	case "warn", "warning":
		currentLog = LogWarn
	case "error":
		currentLog = LogError
	default:
		currentLog = LogInfo
		logln(LogWarn, fmt.Sprintf("Unknown log level '%s', defaulting to 'info'", level))
	}
}

// shouldLog determines if a message at the given level should be logged
func shouldLog(level Log) bool {
	return level >= currentLog
}

// logf - Custom logging function with level prefix and timestamp
// Only logs if the message level meets or exceeds the current log level
func logf(level Log, format string, v ...interface{}) {
	if !shouldLog(level) {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	prefix := fmt.Sprintf("[%s] %s | ", timestamp, level.String())
	formattedMsg := fmt.Sprintf(format, v...)
	fullMsg := prefix + formattedMsg
	log.Print(fullMsg)

	// Add to buffer if available
	if logBuffer != nil {
		logBuffer.Add(fullMsg)
	}
}

// logln - Custom logging function with level prefix for simple messages
// Only logs if the message level meets or exceeds the current log level
func logln(level Log, message string) {
	if !shouldLog(level) {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	prefix := fmt.Sprintf("[%s] %s | ", timestamp, level.String())
	fullMsg := prefix + message
	log.Println(fullMsg)

	// Add to buffer if available
	if logBuffer != nil {
		logBuffer.Add(fullMsg)
	}
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
		logln(LogInfo, "CACHE_DURATION environment variable not set, using default 24 hours")
		return 24 * time.Hour
	}

	// If explicitly set to 0, cache until restart
	if cacheDurationStr == "0" {
		logln(LogInfo, "CACHE_DURATION set to 0, caching until restart")
		return 0
	}

	// Parse the hours value
	hours, err := strconv.Atoi(cacheDurationStr)
	if err != nil {
		logf(LogWarn, "Invalid CACHE_DURATION value: %s, using default 24 hours", cacheDurationStr)
		return 24 * time.Hour
	}

	logf(LogInfo, "CACHE_DURATION set to %d hours", hours)
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

// HTTP handler for health check
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
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

var startTime time.Time

// HTTP handler for dashboard
func dashboardHandler(cacheV4, cacheV6 *ServerInfoCache, serverURLv4, serverURLv6, proxyURLv4, proxyURLv6 string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get stats
		lastReqTime, lastReqIP, totalReqs := requestStats.GetStats()

		// Get cached IPv4 server info
		serverInfoV4 := cacheV4.Get()
		cachedIDv4 := "N/A"
		cachedNamev4 := "N/A"
		cacheAgev4 := "N/A"

		if serverInfoV4 != nil {
			cachedIDv4 = serverInfoV4.Id
			cachedNamev4 = serverInfoV4.ServerName
			age := time.Since(cacheV4.Timestamp)
			cacheAgev4 = age.Round(time.Second).String()
		}

		// Get cached IPv6 server info
		serverInfoV6 := cacheV6.Get()
		cachedIDv6 := "N/A"
		cachedNamev6 := "N/A"
		cacheAgev6 := "N/A"

		if serverInfoV6 != nil {
			cachedIDv6 = serverInfoV6.Id
			cachedNamev6 = serverInfoV6.ServerName
			age := time.Since(cacheV6.Timestamp)
			cacheAgev6 = age.Round(time.Second).String()
		}

		// Format last request time
		lastReqTimeStr := "Never"
		if !lastReqTime.IsZero() {
			lastReqTimeStr = lastReqTime.Format("2006-01-02 15:04:05")
		}

		// Calculate uptime
		uptime := time.Since(startTime).Round(time.Second).String()

		// Get logs
		logs := logBuffer.GetAll()

		data := DashboardData{
			Version:            Version,
			ServerURLv4:        serverURLv4,
			ServerURLv6:        serverURLv6,
			ProxyURLv4:         proxyURLv4,
			ProxyURLv6:         proxyURLv6,
			LastRequestTime:    lastReqTimeStr,
			LastRequestIP:      lastReqIP,
			TotalRequests:      totalReqs,
			CachedServerIDv4:   cachedIDv4,
			CachedServerNamev4: cachedNamev4,
			CacheAgev4:         cacheAgev4,
			CachedServerIDv6:   cachedIDv6,
			CachedServerNamev6: cachedNamev6,
			CacheAgev6:         cacheAgev6,
			BlacklistedIPs:     ipBlacklist.Count(),
			Logs:               logs,
			Uptime:             uptime,
		}

		tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Jellyfin Discovery Proxy Dashboard</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
            background-color: #0d1117;
            color: #c9d1d9;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background-color: #161b22;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.5);
            border: 1px solid #30363d;
        }
        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            border-bottom: 2px solid #21262d;
            padding-bottom: 10px;
            margin-bottom: 20px;
        }
        h1 {
            color: #58a6ff;
            margin: 0;
        }
        .refresh-controls {
            display: flex;
            align-items: center;
            gap: 15px;
        }
        .refresh-button {
            background-color: #238636;
            color: white;
            border: 1px solid #2ea043;
            padding: 8px 16px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 14px;
            font-weight: 500;
            transition: background-color 0.2s;
        }
        .refresh-button:hover {
            background-color: #2ea043;
        }
        .refresh-button:active {
            background-color: #26a641;
        }
        .last-refresh {
            color: #8b949e;
            font-size: 0.9em;
        }
        h2 {
            color: #8b949e;
            margin-top: 30px;
            margin-bottom: 15px;
        }
        p {
            color: #8b949e;
        }
        .info-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 15px;
            margin: 20px 0;
        }
        .info-box {
            background-color: #0d1117;
            padding: 15px;
            border-radius: 6px;
            border-left: 4px solid #58a6ff;
            border: 1px solid #30363d;
        }
        .info-label {
            font-weight: bold;
            color: #8b949e;
            font-size: 0.9em;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .info-value {
            color: #c9d1d9;
            font-size: 1.1em;
            margin-top: 8px;
            font-weight: 500;
        }
        .log-window {
            background-color: #010409;
            color: #7ee787;
            padding: 15px;
            border-radius: 6px;
            font-family: 'Courier New', monospace;
            font-size: 0.85em;
            max-height: 500px;
            overflow-y: auto;
            white-space: pre-wrap;
            word-wrap: break-word;
            border: 1px solid #30363d;
            line-height: 1.5;
        }
        .log-line {
            margin: 2px 0;
        }
        strong {
            color: #58a6ff;
        }
    </style>
    <script>
        let refreshTime = Date.now();

        function updateTimer() {
            const elapsed = Math.floor((Date.now() - refreshTime) / 1000);
            const minutes = Math.floor(elapsed / 60);
            const seconds = elapsed % 60;

            let timeStr = '';
            if (minutes > 0) {
                timeStr = minutes + 'm ' + seconds + 's';
            } else {
                timeStr = seconds + 's';
            }

            document.getElementById('refresh-timer').textContent = 'Since last refresh: ' + timeStr;
        }

        function refreshPage() {
            location.reload();
        }

        // Update timer every second
        setInterval(updateTimer, 1000);
        updateTimer();
    </script>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Jellyfin Discovery Proxy Dashboard</h1>
            <div class="refresh-controls">
                <span class="last-refresh" id="refresh-timer">Since last refresh: 0s</span>
                <button class="refresh-button" onclick="refreshPage()">Refresh</button>
            </div>
        </div>

        <p><strong>Version:</strong> {{.Version}} | <strong>Uptime:</strong> {{.Uptime}}</p>

        <h2>Configuration</h2>
        <div class="info-grid">
            <div class="info-box">
                <div class="info-label">IPv4 Server URL</div>
                <div class="info-value">{{.ServerURLv4}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">IPv6 Server URL</div>
                <div class="info-value">{{.ServerURLv6}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">IPv4 Proxy URL</div>
                <div class="info-value">{{.ProxyURLv4}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">IPv6 Proxy URL</div>
                <div class="info-value">{{.ProxyURLv6}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">Blacklisted IPs</div>
                <div class="info-value">{{.BlacklistedIPs}}</div>
            </div>
        </div>

        <h2>IPv4 Server Information</h2>
        <div class="info-grid">
            <div class="info-box">
                <div class="info-label">Server Name</div>
                <div class="info-value">{{.CachedServerNamev4}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">Server ID</div>
                <div class="info-value">{{.CachedServerIDv4}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">Cache Age</div>
                <div class="info-value">{{.CacheAgev4}}</div>
            </div>
        </div>

        <h2>IPv6 Server Information</h2>
        <div class="info-grid">
            <div class="info-box">
                <div class="info-label">Server Name</div>
                <div class="info-value">{{.CachedServerNamev6}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">Server ID</div>
                <div class="info-value">{{.CachedServerIDv6}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">Cache Age</div>
                <div class="info-value">{{.CacheAgev6}}</div>
            </div>
        </div>

        <h2>Request Statistics</h2>
        <div class="info-grid">
            <div class="info-box">
                <div class="info-label">Last Request Time</div>
                <div class="info-value">{{.LastRequestTime}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">Last Request IP</div>
                <div class="info-value">{{.LastRequestIP}}</div>
            </div>
            <div class="info-box">
                <div class="info-label">Total Requests</div>
                <div class="info-value">{{.TotalRequests}}</div>
            </div>
        </div>

        <h2>Recent Logs</h2>
        <div class="log-window">{{range .Logs}}<div class="log-line">{{.}}</div>{{end}}</div>
    </div>
</body>
</html>`

		t := template.Must(template.New("dashboard").Parse(tmpl))
		w.Header().Set("Content-Type", "text/html")
		t.Execute(w, data)
	}
}

// main()
// Func - Main Function That Initializes and Runs the Jellyfin Discovery Proxy
func main() {
	startTime = time.Now()
	// Parse command-line flags
	logLevelFlag := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Initialize log buffer (default 1024 lines, configurable via environment variable)
	logBufferSize := 1024
	if logBufferSizeStr := os.Getenv("LOG_BUFFER_SIZE"); logBufferSizeStr != "" {
		if size, err := strconv.Atoi(logBufferSizeStr); err == nil && size > 0 {
			logBufferSize = size
		}
	}
	logBuffer = NewLogBuffer(logBufferSize)

	// Check for LOG_LEVEL environment variable, command-line flag takes precedence
	logLevel := *logLevelFlag
	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" && *logLevelFlag == "info" {
		logLevel = envLogLevel
		logf(LogDebug, "Using LOG_LEVEL from environment: %s", envLogLevel)
	}

	// Set log level
	setLog(logLevel)

	// Initialize IP blacklist
	blacklistStr := os.Getenv("BLACKLIST")
	ipBlacklist = NewIPBlacklist(blacklistStr)
	if ipBlacklist.Count() > 0 {
		logf(LogInfo, "Loaded %d IP(s) into blacklist", ipBlacklist.Count())
	}

	logln(LogInfo, "=== Jellyfin Discovery Proxy Starting ===")
	logf(LogInfo, "Version: %s", Version)
	logf(LogDebug, "Log level set to: %s", currentLog.String())

	// Get server URLs from environment variables
	// Support both legacy JELLYFIN_SERVER_URL and new IPv4/IPv6 specific URLs
	serverURL := os.Getenv("JELLYFIN_SERVER_URL")
	serverURLv4 := os.Getenv("JELLYFIN_SERVER_URL_IPV4")
	serverURLv6 := os.Getenv("JELLYFIN_SERVER_URL_IPV6")

	// Determine which URLs to use
	if serverURLv4 == "" && serverURLv6 == "" {
		// Legacy mode: use JELLYFIN_SERVER_URL for both
		if serverURL == "" {
			logln(LogInfo, "No server URL environment variables set, using default http://localhost:8096")
			serverURL = "http://localhost:8096"
		}
		serverURLv4 = serverURL
		serverURLv6 = serverURL
		logf(LogDebug, "Using legacy mode - both IPv4 and IPv6 will use: '%s'", serverURL)
	} else {
		// New mode: use separate URLs for IPv4 and IPv6
		if serverURLv4 == "" && serverURLv6 == "" {
			logf(LogError, "At least one of JELLYFIN_SERVER_URL_IPV4 or JELLYFIN_SERVER_URL_IPV6 must be set")
			os.Exit(1)
		}
		if serverURLv4 == "" {
			serverURLv4 = serverURLv6
			logf(LogInfo, "JELLYFIN_SERVER_URL_IPV4 not set, using IPv6 URL for IPv4: %s", serverURLv4)
		}
		if serverURLv6 == "" {
			serverURLv6 = serverURLv4
			logf(LogInfo, "JELLYFIN_SERVER_URL_IPV6 not set, using IPv4 URL for IPv6: %s", serverURLv6)
		}
		logf(LogDebug, "IPv4 server URL: '%s'", serverURLv4)
		logf(LogDebug, "IPv6 server URL: '%s'", serverURLv6)
	}

	// Get proxy URL from environment variable
	proxyURL := os.Getenv("PROXY_URL")
	proxyURLv4 := os.Getenv("PROXY_URL_IPV4")
	proxyURLv6 := os.Getenv("PROXY_URL_IPV6")

	// Determine which proxy URLs to use
	if proxyURLv4 == "" && proxyURLv6 == "" {
		// Legacy mode: use PROXY_URL for both
		if proxyURL == "" {
			logln(LogInfo, "PROXY_URL environment variable not set, will use JELLYFIN_SERVER_URL for Address field")
			proxyURLv4 = serverURLv4
			proxyURLv6 = serverURLv6
		} else {
			proxyURLv4 = proxyURL
			proxyURLv6 = proxyURL
			logf(LogInfo, "PROXY_URL set to %s, will use for Address field in responses", proxyURL)
			logf(LogDebug, "Using legacy proxy URL mode - both IPv4 and IPv6 will use: '%s'", proxyURL)
			if isHostname(proxyURL) {
				logln(LogInfo, "PROXY_URL is a hostname, will broadcast both hostname and IP responses for non-Avahi device compatibility")
				logf(LogDebug, "Detected hostname in PROXY_URL: %s", proxyURL)
			}
		}
	} else {
		// New mode: use separate proxy URLs
		if proxyURLv4 == "" {
			proxyURLv4 = serverURLv4
			logf(LogDebug, "PROXY_URL_IPV4 not set, using server URL: %s", proxyURLv4)
		}
		if proxyURLv6 == "" {
			proxyURLv6 = serverURLv6
			logf(LogDebug, "PROXY_URL_IPV6 not set, using server URL: %s", proxyURLv6)
		}
		logf(LogInfo, "IPv4 proxy URL: %s", proxyURLv4)
		logf(LogInfo, "IPv6 proxy URL: %s", proxyURLv6)
		if isHostname(proxyURLv4) {
			logf(LogDebug, "IPv4 proxy URL is a hostname: %s", proxyURLv4)
		}
		if isHostname(proxyURLv6) {
			logf(LogDebug, "IPv6 proxy URL is a hostname: %s", proxyURLv6)
		}
	}

	// Remove trailing slash if present from URLs
	serverURLv4 = strings.TrimSuffix(serverURLv4, "/")
	serverURLv6 = strings.TrimSuffix(serverURLv6, "/")
	proxyURLv4 = strings.TrimSuffix(proxyURLv4, "/")
	proxyURLv6 = strings.TrimSuffix(proxyURLv6, "/")
	logf(LogDebug, "Processed IPv4 - serverURL: '%s', proxyURL: '%s'", serverURLv4, proxyURLv4)
	logf(LogDebug, "Processed IPv6 - serverURL: '%s', proxyURL: '%s'", serverURLv6, proxyURLv6)

	logf(LogInfo, "Target Jellyfin server IPv4: %s", serverURLv4)
	logf(LogInfo, "Target Jellyfin server IPv6: %s", serverURLv6)

	// Determine cache duration
	cacheDuration := GetCacheDuration()

	// Get network interface if specified
	networkInterface := os.Getenv("NETWORK_INTERFACE")
	var bindIP string
	if networkInterface != "" {
		logf(LogInfo, "NETWORK_INTERFACE set to: %s", networkInterface)
		iface, err := net.InterfaceByName(networkInterface)
		if err != nil {
			logf(LogError, "Failed to find network interface '%s': %v", networkInterface, err)
			os.Exit(1)
		}

		addrs, err := iface.Addrs()
		if err != nil {
			logf(LogError, "Failed to get addresses for interface '%s': %v", networkInterface, err)
			os.Exit(1)
		}

		// Find first IPv4 address
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					bindIP = ipnet.IP.String()
					logf(LogInfo, "Binding to interface %s with IP: %s", networkInterface, bindIP)
					break
				}
			}
		}

		if bindIP == "" {
			logf(LogError, "No IPv4 address found on interface '%s'", networkInterface)
			os.Exit(1)
		}
	} else {
		bindIP = "0.0.0.0"
		logln(LogInfo, "No NETWORK_INTERFACE specified, binding to all interfaces")
	}

	// Create UDP listeners (udp6 if available, plus udp4)
	var conns []*net.UDPConn

	// Try IPv6 first if binding to all interfaces (this may be dual-stack on some OSes)
	if bindIP == "0.0.0.0" {
		logf(LogDebug, "Attempting to bind UDP6 listener on [::]:7359")
		addr6, err := net.ResolveUDPAddr("udp6", "[::]:7359")
		if err != nil {
			logf(LogWarn, "Error resolving UDP6 address: %v", err)
			logf(LogDebug, "UDP6 resolution failed with error type: %T", err)
		} else if conn6, err := net.ListenUDP("udp6", addr6); err != nil {
			logf(LogWarn, "UDP6 not available on port 7359: %v", err)
			logf(LogDebug, "UDP6 bind failed with error type: %T", err)
		} else {
			conns = append(conns, conn6)
			logln(LogInfo, "Successfully bound to UDP6 [::]:7359 for discovery requests")
			logf(LogDebug, "UDP6 connection local address: %s", conn6.LocalAddr())
		}
	}

	// Always try IPv4
	udp4Addr := fmt.Sprintf("%s:7359", bindIP)
	logf(LogDebug, "Attempting to bind UDP4 listener on %s", udp4Addr)
	addr4, err := net.ResolveUDPAddr("udp4", udp4Addr)
	if err != nil {
		logf(LogError, "Error resolving UDP4 address: %v", err)
		logf(LogDebug, "UDP4 resolution failed with error type: %T", err)
		os.Exit(1)
	}
	if conn4, err := net.ListenUDP("udp4", addr4); err != nil {
		if len(conns) == 0 {
			logf(LogError, "Error listening on UDP4 port 7359: %v", err)
			logf(LogDebug, "UDP4 bind failed with error type: %T", err)
			os.Exit(1)
		}
		// On some systems, the UDP6 socket may already be dual-stack and occupy the port.
		logf(LogWarn, "Could not bind UDP4 (possibly already covered by UDP6): %v", err)
		logf(LogDebug, "UDP4 bind failed with error type: %T", err)
	} else {
		conns = append(conns, conn4)
		logf(LogInfo, "Successfully bound to UDP4 %s for discovery requests", udp4Addr)
		logf(LogDebug, "UDP4 connection local address: %s", conn4.LocalAddr())
	}

	if len(conns) == 0 {
		logln(LogError, "No UDP listeners could be created on port 7359")
		os.Exit(1)
	}

	logf(LogDebug, "Total active UDP listeners: %d", len(conns))

	for _, c := range conns {
		defer c.Close()
	}

	if cacheDuration == 0 {
		logln(LogInfo, "Server info will be cached until restart")
	} else {
		logf(LogInfo, "Server info will be cached for %v", cacheDuration)
	}
	logf(LogDebug, "Cache duration in nanoseconds: %d", cacheDuration.Nanoseconds())

	// Initialize caches with determined duration (one for IPv4, one for IPv6)
	cacheV4 := NewServerInfoCache(cacheDuration)
	cacheV6 := NewServerInfoCache(cacheDuration)
	logf(LogDebug, "Initialized server info caches with duration: %v", cacheDuration)

	// Fetch server info once at startup for IPv4
	logf(LogDebug, "Attempting initial IPv4 server info fetch from %s", serverURLv4)
	serverInfoV4, err := fetchServerInfo(serverURLv4)
	if err != nil {
		logf(LogWarn, "Could not fetch IPv4 server info at startup: %v", err)
		logln(LogWarn, "Will try again when IPv4 discovery requests are received")
		logf(LogDebug, "IPv4 startup fetch failed with error type: %T", err)
	} else {
		logf(LogInfo, "Successfully fetched IPv4 server info - ID: %s, Name: %s", serverInfoV4.Id, serverInfoV4.ServerName)
		cacheV4.Set(serverInfoV4)
		logf(LogDebug, "IPv4 server info cached at: %v", cacheV4.Timestamp)
	}

	// Fetch server info once at startup for IPv6 (only if different from IPv4)
	if serverURLv6 != serverURLv4 {
		logf(LogDebug, "Attempting initial IPv6 server info fetch from %s", serverURLv6)
		serverInfoV6, err := fetchServerInfo(serverURLv6)
		if err != nil {
			logf(LogWarn, "Could not fetch IPv6 server info at startup: %v", err)
			logln(LogWarn, "Will try again when IPv6 discovery requests are received")
			logf(LogDebug, "IPv6 startup fetch failed with error type: %T", err)
		} else {
			logf(LogInfo, "Successfully fetched IPv6 server info - ID: %s, Name: %s", serverInfoV6.Id, serverInfoV6.ServerName)
			cacheV6.Set(serverInfoV6)
			logf(LogDebug, "IPv6 server info cached at: %v", cacheV6.Timestamp)
		}
	} else {
		// Same URL for both, share the cache entry
		if serverInfoV4 != nil {
			cacheV6.Set(serverInfoV4)
			logf(LogDebug, "IPv6 using same cache as IPv4")
		}
	}

	// Start HTTP server on port 8080
	httpServer := &http.Server{
		Addr: ":8080",
	}

	http.HandleFunc("/health", healthCheckHandler)
	http.HandleFunc("/", dashboardHandler(cacheV4, cacheV6, serverURLv4, serverURLv6, proxyURLv4, proxyURLv6))

	go func() {
		logln(LogInfo, "Starting HTTP server on port 8080")
		logln(LogInfo, "Dashboard available at http://localhost:8080")
		logln(LogInfo, "Health check available at http://localhost:8080/health")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logf(LogError, "HTTP server error: %v", err)
		}
	}()

	logln(LogInfo, "=== Jellyfin Discovery Proxy Ready ===")
	logf(LogDebug, "Starting %d listener goroutines", len(conns))

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Listen for incoming requests on all sockets
	for i, c := range conns {
		logf(LogDebug, "Starting listener goroutine %d for %s", i, c.LocalAddr())
		// Determine if this is IPv4 or IPv6 listener
		isIPv6 := strings.Contains(c.LocalAddr().String(), "[")
		if isIPv6 {
			logf(LogDebug, "Listener %d is IPv6, using IPv6 URLs and cache", i)
			go listenLoop(ctx, c, serverURLv6, proxyURLv6, cacheV6)
		} else {
			logf(LogDebug, "Listener %d is IPv4, using IPv4 URLs and cache", i)
			go listenLoop(ctx, c, serverURLv4, proxyURLv4, cacheV4)
		}
	}

	logln(LogDebug, "Main thread waiting for shutdown signal")

	// Wait for shutdown signal
	sig := <-sigChan
	logf(LogInfo, "Received signal %v, initiating graceful shutdown", sig)

	// Cancel context to signal goroutines to stop
	cancel()

	// Shutdown HTTP server
	logln(LogInfo, "Shutting down HTTP server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logf(LogWarn, "HTTP server shutdown error: %v", err)
	}

	// Close all UDP connections
	logln(LogInfo, "Closing UDP listeners")
	for i, c := range conns {
		logf(LogDebug, "Closing UDP listener %d: %s", i, c.LocalAddr())
		c.Close()
	}

	// Give goroutines a moment to finish
	time.Sleep(100 * time.Millisecond)

	logln(LogInfo, "=== Jellyfin Discovery Proxy Stopped ===")
	os.Exit(0)
}

// listenLoop()
// Func - Listens for discovery requests on a single UDP socket
func listenLoop(ctx context.Context, conn *net.UDPConn, serverURL, proxyURL string, cache *ServerInfoCache) {
	buffer := make([]byte, 1024)
	logf(LogDebug, "Listener started for %s with buffer size: %d bytes", conn.LocalAddr(), len(buffer))

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			logf(LogDebug, "Context cancelled, stopping listener for %s", conn.LocalAddr())
			return
		default:
		}

		// Set a read deadline so we can check context periodically
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		logf(LogDebug, "Waiting for UDP packet on %s", conn.LocalAddr())
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			// Check if it's a timeout error (normal during shutdown check)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Check if connection is closed (during shutdown)
			if ctx.Err() != nil {
				logf(LogDebug, "Connection closed during shutdown: %s", conn.LocalAddr())
				return
			}
			logf(LogError, "Error reading UDP message: %v", err)
			logf(LogDebug, "UDP read error type: %T, connection: %s", err, conn.LocalAddr())
			continue
		}

		message := string(buffer[:n])
		logf(LogInfo, "Received discovery request from %s (%d bytes): %s", addr.String(), n, message)
		logf(LogDebug, "Message hex dump: % X", buffer[:n])
		logf(LogDebug, "Remote address details - IP: %s, Port: %d, Zone: %s", addr.IP, addr.Port, addr.Zone)

		// Check if this is a Jellyfin discovery request
		if strings.EqualFold(message, "Who is JellyfinServer?") {
			logf(LogDebug, "Valid Jellyfin discovery request detected, spawning handler goroutine")
			go handleDiscoveryRequest(conn, addr, serverURL, proxyURL, cache)
		} else {
			logf(LogWarn, "Ignoring unrecognized message from %s: %s", addr.String(), message)
			logf(LogDebug, "Expected 'Who is JellyfinServer?' but got '%s'", message)
		}
	}
}

// handleDiscoveryRequest()
// Func - Processes Jellyfin Discovery Requests and Sends Appropriate Responses
func handleDiscoveryRequest(conn *net.UDPConn, addr *net.UDPAddr, serverURL string, proxyURL string, cache *ServerInfoCache) {
	logf(LogInfo, "Processing discovery request from %s", addr.String())
	logf(LogDebug, "Handler goroutine started for request from %s", addr.String())

	// Check if IP is blacklisted
	clientIP := addr.IP.String()
	if ipBlacklist.IsBlocked(clientIP) {
		logf(LogWarn, "Ignoring request from blacklisted IP: %s", clientIP)
		return
	}

	// Record request statistics
	requestStats.RecordRequest(clientIP)

	// Try to get info from cache first
	logf(LogDebug, "Checking cache for server info")
	serverInfo := cache.Get()

	// If cache is empty or expired, fetch fresh info
	if serverInfo == nil {
		logln(LogInfo, "Cache expired or empty, fetching fresh server info from Jellyfin")
		logf(LogDebug, "Cache miss - last cached at: %v, cache duration: %v", cache.Timestamp, cache.Duration)

		var err error
		serverInfo, err = fetchServerInfo(serverURL)
		if err != nil {
			logf(LogError, "Failed to fetch server info: %v", err)
			logf(LogDebug, "Fetch error type: %T", err)
			logf(LogWarn, "Not responding to discovery request from %s - server is unreachable", addr.String())
			return // Don't respond if server is unreachable
		}

		// Update cache with new info
		cache.Set(serverInfo)
		logln(LogInfo, "Successfully updated cache with fresh server info")
		logf(LogDebug, "Cache updated at: %v", cache.Timestamp)
	} else {
		logln(LogInfo, "Using cached server info for response")
		cacheAge := time.Since(cache.Timestamp)
		logf(LogDebug, "Cache hit - age: %v, cached at: %v", cacheAge, cache.Timestamp)
	}

	// Determine which URL to use for the Address field
	addressURL := ""

	if proxyURL != "" {
		addressURL = proxyURL
		logf(LogDebug, "Using PROXY_URL for address: %s", addressURL)
	} else {
		addressURL = serverURL
		logf(LogDebug, "Using JELLYFIN_SERVER_URL for address: %s", addressURL)
	}

	// If proxy URL is a hostname, send two responses: hostname and IP
	if proxyURL != "" && isHostname(proxyURL) {
		logln(LogInfo, "Sending dual responses (hostname + IP) for non-Avahi device compatibility")
		logf(LogDebug, "Dual response mode enabled for hostname: %s", proxyURL)

		// Send first response with hostname
		sendDiscoveryResponse(conn, addr, addressURL, serverInfo)

		// Try to resolve hostname to IP and send second response
		logf(LogDebug, "Attempting to resolve hostname %s to IP", proxyURL)
		ipURL, err := resolveHostnameToIP(proxyURL)
		if err != nil {
			logf(LogWarn, "Could not resolve hostname %s to IP: %v", proxyURL, err)
			logf(LogDebug, "DNS resolution error type: %T", err)
		} else {
			logf(LogInfo, "Resolved %s to %s, sending second response", proxyURL, ipURL)
			logf(LogDebug, "Hostname resolved successfully to: %s", ipURL)
			sendDiscoveryResponse(conn, addr, ipURL, serverInfo)
		}
	} else {
		// Send single response
		logf(LogDebug, "Single response mode - sending one discovery response")
		sendDiscoveryResponse(conn, addr, addressURL, serverInfo)
	}

	logf(LogDebug, "Handler goroutine completed for %s", addr.String())
}

// MARK: sendDiscoveryResponse()
// Func - Sends a single discovery response with the given address URL
func sendDiscoveryResponse(conn *net.UDPConn, addr *net.UDPAddr, addressURL string, serverInfo *SystemInfoResponse) {
	logf(LogDebug, "Constructing discovery response for %s", addr.String())

	// Create response
	response := JellyfinDiscoveryResponse{
		Address:         addressURL,
		Id:              serverInfo.Id,
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}
	logf(LogDebug, "Response struct - Address: %s, Id: %s, Name: %s", response.Address, response.Id, response.Name)

	// Serialize to JSON
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		logf(LogError, "Error marshaling JSON response: %v", err)
		logf(LogDebug, "JSON marshal error type: %T", err)
		return
	}
	logf(LogDebug, "JSON response length: %d bytes", len(jsonResponse))
	logf(LogDebug, "JSON response content: %s", string(jsonResponse))

	// Send response
	bytesWritten, err := conn.WriteToUDP(jsonResponse, addr)
	if err != nil {
		logf(LogError, "Error sending response to %s: %v", addr.String(), err)
		logf(LogDebug, "UDP write error type: %T", err)
		return
	}

	logf(LogInfo, "Sent discovery response to %s | Server: %s | Address: %s", addr.String(), serverInfo.ServerName, addressURL)
	logf(LogDebug, "Successfully sent %d bytes to %s", bytesWritten, addr.String())
}

// fetchServerInfo()
// Func - Retrieves Server Information from Jellyfin System/Info Endpoint
func fetchServerInfo(serverURL string) (*SystemInfoResponse, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	logf(LogDebug, "Created HTTP client with timeout: 5s")

	// Call the Jellyfin system info endpoint
	infoURL := fmt.Sprintf("%s/System/Info/Public", serverURL)
	logf(LogInfo, "Fetching server info from: %s", infoURL)
	logf(LogDebug, "Making HTTP GET request to: %s", infoURL)

	resp, err := client.Get(infoURL)
	if err != nil {
		logf(LogDebug, "HTTP request error type: %T", err)
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	logf(LogDebug, "HTTP response status: %d %s", resp.StatusCode, resp.Status)
	logf(LogDebug, "HTTP response headers: %v", resp.Header)

	if resp.StatusCode != http.StatusOK {
		logf(LogDebug, "Non-OK status code received: %d", resp.StatusCode)
		return nil, fmt.Errorf("HTTP request returned status %d", resp.StatusCode)
	}

	// Parse response
	var serverInfo SystemInfoResponse
	logf(LogDebug, "Attempting to decode JSON response body")
	err = json.NewDecoder(resp.Body).Decode(&serverInfo)
	if err != nil {
		logf(LogDebug, "JSON decode error type: %T", err)
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	logf(LogInfo, "Successfully retrieved server info from API (Server: %s, ID: %s)", serverInfo.ServerName, serverInfo.Id)
	logf(LogDebug, "Decoded server info - ServerName: '%s', Id: '%s'", serverInfo.ServerName, serverInfo.Id)
	return &serverInfo, nil
}
