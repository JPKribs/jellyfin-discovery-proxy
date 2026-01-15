package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/blacklist"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/cache"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/config"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/discovery"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/logging"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/server"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/stats"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/web"
)

func main() {
	web.StartTime = time.Now()

	// Parse command-line flags
	logLevelFlag := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Initialize log buffer
	logging.LogBuffer = logging.NewLogBuffer(logging.GetLogBufferSize())

	// Set log level from flag or environment
	logLevel := *logLevelFlag
	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" && *logLevelFlag == "info" {
		logLevel = envLogLevel
		logging.Logf(types.LogDebug, "Using LOG_LEVEL from environment: %s", envLogLevel)
	}
	logging.SetLog(logLevel)

	// Initialize IP blacklist
	blacklistStr := os.Getenv("BLACKLIST")
	ipBlacklist := blacklist.New(blacklistStr)
	if ipBlacklist.Count() > 0 {
		logging.Logf(types.LogInfo, "Loaded %d IP(s) into blacklist", ipBlacklist.Count())
	}

	// Initialize request stats
	requestStats := stats.New()

	logging.Logln(types.LogInfo, "=== Jellyfin Discovery Proxy Starting ===")
	logging.Logf(types.LogInfo, "Version: %s", types.Version)
	logging.Logf(types.LogDebug, "Log level set to: %s", logging.CurrentLog.String())

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logging.Logf(types.LogError, "Configuration error: %v", err)
		os.Exit(1)
	}

	// Determine cache duration
	cacheDuration := cache.GetDuration()

	// Create UDP listeners
	conns, err := createUDPListeners(cfg.BindIP)
	if err != nil {
		logging.Logf(types.LogError, "Failed to create UDP listeners: %v", err)
		os.Exit(1)
	}
	defer closeConnections(conns)

	if cacheDuration == 0 {
		logging.Logln(types.LogInfo, "Server info will be cached until restart")
	} else {
		logging.Logf(types.LogInfo, "Server info will be cached for %v", cacheDuration)
	}
	logging.Logf(types.LogDebug, "Cache duration in nanoseconds: %d", cacheDuration.Nanoseconds())

	// Initialize caches
	cacheV4 := cache.New(cacheDuration)
	cacheV6 := cache.New(cacheDuration)
	logging.Logf(types.LogDebug, "Initialized server info caches with duration: %v", cacheDuration)

	// Fetch initial server info
	fetchInitialServerInfo(cfg.ServerURLv4, cfg.ServerURLv6, cacheV4, cacheV6)

	// Start HTTP server
	httpServer := startHTTPServer(cacheV4, cacheV6, cfg, requestStats, ipBlacklist)

	logging.Logln(types.LogInfo, "=== Jellyfin Discovery Proxy Ready ===")
	logging.Logf(types.LogDebug, "Starting %d listener goroutines", len(conns))

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Start listeners
	startListeners(ctx, conns, cfg, cacheV4, cacheV6, ipBlacklist, requestStats)

	logging.Logln(types.LogDebug, "Main thread waiting for shutdown signal")

	// Wait for shutdown signal
	sig := <-sigChan
	logging.Logf(types.LogInfo, "Received signal %v, initiating graceful shutdown", sig)

	// Perform graceful shutdown
	gracefulShutdown(cancel, httpServer, conns)

	logging.Logln(types.LogInfo, "=== Jellyfin Discovery Proxy Stopped ===")
	os.Exit(0)
}

// createUDPListeners creates IPv4 and IPv6 UDP listeners
func createUDPListeners(bindIP string) ([]*net.UDPConn, error) {
	var conns []*net.UDPConn

	// Try IPv6 first if binding to all interfaces
	if bindIP == "0.0.0.0" {
		logging.Logf(types.LogDebug, "Attempting to bind UDP6 listener on [::]:7359")
		addr6, err := net.ResolveUDPAddr("udp6", "[::]:7359")
		if err != nil {
			logging.Logf(types.LogWarn, "Error resolving UDP6 address: %v", err)
			logging.Logf(types.LogDebug, "UDP6 resolution failed with error type: %T", err)
		} else if conn6, err := net.ListenUDP("udp6", addr6); err != nil {
			logging.Logf(types.LogWarn, "UDP6 not available on port 7359: %v", err)
			logging.Logf(types.LogDebug, "UDP6 bind failed with error type: %T", err)
		} else {
			conns = append(conns, conn6)
			logging.Logln(types.LogInfo, "Successfully bound to UDP6 [::]:7359 for discovery requests")
			logging.Logf(types.LogDebug, "UDP6 connection local address: %s", conn6.LocalAddr())
		}
	}

	// Always try IPv4
	udp4Addr := fmt.Sprintf("%s:7359", bindIP)
	logging.Logf(types.LogDebug, "Attempting to bind UDP4 listener on %s", udp4Addr)
	addr4, err := net.ResolveUDPAddr("udp4", udp4Addr)
	if err != nil {
		logging.Logf(types.LogError, "Error resolving UDP4 address: %v", err)
		logging.Logf(types.LogDebug, "UDP4 resolution failed with error type: %T", err)
		return nil, fmt.Errorf("failed to resolve UDP4 address: %v", err)
	}

	if conn4, err := net.ListenUDP("udp4", addr4); err != nil {
		if len(conns) == 0 {
			logging.Logf(types.LogError, "Error listening on UDP4 port 7359: %v", err)
			logging.Logf(types.LogDebug, "UDP4 bind failed with error type: %T", err)
			return nil, fmt.Errorf("failed to bind UDP4: %v", err)
		}
		logging.Logf(types.LogWarn, "Could not bind UDP4 (possibly already covered by UDP6): %v", err)
		logging.Logf(types.LogDebug, "UDP4 bind failed with error type: %T", err)
	} else {
		conns = append(conns, conn4)
		logging.Logf(types.LogInfo, "Successfully bound to UDP4 %s for discovery requests", udp4Addr)
		logging.Logf(types.LogDebug, "UDP4 connection local address: %s", conn4.LocalAddr())
	}

	if len(conns) == 0 {
		return nil, fmt.Errorf("no UDP listeners could be created on port 7359")
	}

	logging.Logf(types.LogDebug, "Total active UDP listeners: %d", len(conns))
	return conns, nil
}

// fetchInitialServerInfo fetches server info at startup
func fetchInitialServerInfo(serverURLv4, serverURLv6 string, cacheV4, cacheV6 *types.ServerInfoCache) {
	// Fetch IPv4
	logging.Logf(types.LogDebug, "Attempting initial IPv4 server info fetch from %s", serverURLv4)
	serverInfoV4, err := server.FetchInfo(serverURLv4)
	if err != nil {
		logging.Logf(types.LogWarn, "Could not fetch IPv4 server info at startup: %v", err)
		logging.Logln(types.LogWarn, "Will try again when IPv4 discovery requests are received")
		logging.Logf(types.LogDebug, "IPv4 startup fetch failed with error type: %T", err)
	} else {
		logging.Logf(types.LogInfo, "Successfully fetched IPv4 server info - ID: %s, Name: %s", serverInfoV4.Id, serverInfoV4.ServerName)
		cacheV4.Set(serverInfoV4)
		logging.Logf(types.LogDebug, "IPv4 server info cached at: %v", cacheV4.Timestamp)
	}

	// Fetch IPv6 if different from IPv4
	if serverURLv6 != serverURLv4 {
		logging.Logf(types.LogDebug, "Attempting initial IPv6 server info fetch from %s", serverURLv6)
		serverInfoV6, err := server.FetchInfo(serverURLv6)
		if err != nil {
			logging.Logf(types.LogWarn, "Could not fetch IPv6 server info at startup: %v", err)
			logging.Logln(types.LogWarn, "Will try again when IPv6 discovery requests are received")
			logging.Logf(types.LogDebug, "IPv6 startup fetch failed with error type: %T", err)
		} else {
			logging.Logf(types.LogInfo, "Successfully fetched IPv6 server info - ID: %s, Name: %s", serverInfoV6.Id, serverInfoV6.ServerName)
			cacheV6.Set(serverInfoV6)
			logging.Logf(types.LogDebug, "IPv6 server info cached at: %v", cacheV6.Timestamp)
		}
	} else {
		// Same URL for both, share the cache entry
		if serverInfoV4 != nil {
			cacheV6.Set(serverInfoV4)
			logging.Logf(types.LogDebug, "IPv6 using same cache as IPv4")
		}
	}
}

// startHTTPServer starts the HTTP server for the dashboard
func startHTTPServer(cacheV4, cacheV6 *types.ServerInfoCache, cfg *types.Config, requestStats *types.RequestStats, ipBlacklist *types.IPBlacklist) *http.Server {
	httpServer := &http.Server{
		Addr: fmt.Sprintf(":%s", cfg.HTTPPort),
	}

	http.HandleFunc("/health", web.HealthCheckHandler)
	http.HandleFunc("/", web.DashboardHandler(cacheV4, cacheV6, cfg.ServerURLv4, cfg.ServerURLv6, cfg.ProxyURLv4, cfg.ProxyURLv6, requestStats, ipBlacklist, logging.LogBuffer, types.Version))
	http.HandleFunc("/static/", web.StaticFileHandler)

	go func() {
		logging.Logf(types.LogInfo, "Starting HTTP server on port %s", cfg.HTTPPort)
		logging.Logf(types.LogInfo, "Dashboard available at http://localhost:%s", cfg.HTTPPort)
		logging.Logf(types.LogInfo, "Health check available at http://localhost:%s/health", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Logf(types.LogError, "HTTP server error: %v", err)
		}
	}()

	return httpServer
}

// startListeners starts all UDP listener goroutines
func startListeners(ctx context.Context, conns []*net.UDPConn, cfg *types.Config, cacheV4, cacheV6 *types.ServerInfoCache, ipBlacklist *types.IPBlacklist, requestStats *types.RequestStats) {
	for i, c := range conns {
		logging.Logf(types.LogDebug, "Starting listener goroutine %d for %s", i, c.LocalAddr())
		// Determine if this is IPv4 or IPv6 listener
		isIPv6 := strings.Contains(c.LocalAddr().String(), "[")
		if isIPv6 {
			logging.Logf(types.LogDebug, "Listener %d is IPv6, using IPv6 URLs and cache", i)
			go discovery.ListenLoop(ctx, c, cfg.ServerURLv6, cfg.ProxyURLv6, cacheV6, ipBlacklist, requestStats)
		} else {
			logging.Logf(types.LogDebug, "Listener %d is IPv4, using IPv4 URLs and cache", i)
			go discovery.ListenLoop(ctx, c, cfg.ServerURLv4, cfg.ProxyURLv4, cacheV4, ipBlacklist, requestStats)
		}
	}
}

// gracefulShutdown performs graceful shutdown of all services
func gracefulShutdown(cancel context.CancelFunc, httpServer *http.Server, conns []*net.UDPConn) {
	// Cancel context to signal goroutines to stop
	cancel()

	// Shutdown HTTP server
	logging.Logln(types.LogInfo, "Shutting down HTTP server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logging.Logf(types.LogWarn, "HTTP server shutdown error: %v", err)
	}

	// Close all UDP connections
	closeConnections(conns)

	// Give goroutines a moment to finish
	time.Sleep(100 * time.Millisecond)
}

// closeConnections closes all UDP connections
func closeConnections(conns []*net.UDPConn) {
	logging.Logln(types.LogInfo, "Closing UDP listeners")
	for i, c := range conns {
		logging.Logf(types.LogDebug, "Closing UDP listener %d: %s", i, c.LocalAddr())
		c.Close()
	}
}
