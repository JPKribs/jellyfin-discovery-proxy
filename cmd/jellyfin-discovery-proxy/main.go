package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/blacklist"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/cache"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/config"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/discovery"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/hooks"
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
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(types.Version)
		os.Exit(0)
	}

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

	// Load hook configuration
	hookConfig := hooks.LoadHookConfig()
	if hookConfig.OnReceiveURL != "" || hookConfig.OnReceiveCmd != "" {
		logging.Logf(types.LogInfo, "onReceive hook configured")
		logging.Logf(types.LogDebug, "onReceive URL: %s, CMD: %s", hookConfig.OnReceiveURL, hookConfig.OnReceiveCmd)
	}
	if hookConfig.OnSendURL != "" || hookConfig.OnSendCmd != "" {
		logging.Logf(types.LogInfo, "onSend hook configured")
		logging.Logf(types.LogDebug, "onSend URL: %s, CMD: %s", hookConfig.OnSendURL, hookConfig.OnSendCmd)
	}

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

	// Create the UDP listener (IPv4 only — Jellyfin discovery is an IPv4 broadcast).
	conn, err := createUDPListener(cfg.BindIP)
	if err != nil {
		logging.Logf(types.LogError, "Failed to create UDP listener: %v", err)
		os.Exit(1)
	}

	if cacheDuration == 0 {
		logging.Logln(types.LogInfo, "Server info will be cached until restart")
	} else {
		logging.Logf(types.LogInfo, "Server info will be cached for %v", cacheDuration)
	}
	logging.Logf(types.LogDebug, "Cache duration in nanoseconds: %d", cacheDuration.Nanoseconds())

	// Initialize cache
	serverCache := cache.New(cacheDuration)
	logging.Logf(types.LogDebug, "Initialized server info cache with duration: %v", cacheDuration)

	// Fetch initial server info
	fetchInitialServerInfo(cfg.ServerURL, serverCache)

	// Start HTTP server
	httpServer := startHTTPServer(serverCache, cfg, requestStats, ipBlacklist)

	logging.Logln(types.LogInfo, "=== Jellyfin Discovery Proxy Ready ===")

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the listener
	startListener(ctx, conn, cfg, serverCache, ipBlacklist, requestStats, hookConfig)

	logging.Logln(types.LogDebug, "Main thread waiting for shutdown signal")

	// Wait for shutdown signal
	sig := <-sigChan
	logging.Logf(types.LogInfo, "Received signal %v, initiating graceful shutdown", sig)

	// Perform graceful shutdown
	gracefulShutdown(cancel, httpServer, conn)

	logging.Logln(types.LogInfo, "=== Jellyfin Discovery Proxy Stopped ===")
	os.Exit(0)
}

// createUDPListener creates the IPv4 UDP listener for Jellyfin discovery.
// Jellyfin clients broadcast on 255.255.255.255:7359, which is IPv4-only —
// IPv6 has no broadcast equivalent, so a v6 socket would never receive a
// real discovery request.
func createUDPListener(bindIP string) (*net.UDPConn, error) {
	udp4Addr := fmt.Sprintf("%s:%d", bindIP, types.DiscoveryPort)
	logging.Logf(types.LogDebug, "Attempting to bind UDP4 listener on %s", udp4Addr)
	addr4, err := net.ResolveUDPAddr("udp4", udp4Addr)
	if err != nil {
		logging.Logf(types.LogError, "Error resolving UDP4 address: %v", err)
		logging.Logf(types.LogDebug, "UDP4 resolution failed with error type: %T", err)
		return nil, fmt.Errorf("failed to resolve UDP4 address: %v", err)
	}

	conn4, err := net.ListenUDP("udp4", addr4)
	if err != nil {
		logging.Logf(types.LogError, "Error listening on UDP4 port %d: %v", types.DiscoveryPort, err)
		logging.Logf(types.LogDebug, "UDP4 bind failed with error type: %T", err)
		return nil, fmt.Errorf("failed to bind UDP4: %v", err)
	}

	logging.Logf(types.LogInfo, "Successfully bound to UDP4 %s for discovery requests", udp4Addr)
	logging.Logf(types.LogDebug, "UDP4 connection local address: %s", conn4.LocalAddr())
	return conn4, nil
}

// fetchInitialServerInfo fetches server info at startup so the first
// discovery request doesn't pay the full HTTP roundtrip.
func fetchInitialServerInfo(serverURL string, serverCache *types.ServerInfoCache) {
	logging.Logf(types.LogDebug, "Attempting initial server info fetch from %s", serverURL)
	serverInfo, err := server.FetchInfo(serverURL)
	if err != nil {
		logging.Logf(types.LogWarn, "Could not fetch server info at startup: %v", err)
		logging.Logln(types.LogWarn, "Will try again when discovery requests are received")
		logging.Logf(types.LogDebug, "Startup fetch failed with error type: %T", err)
		return
	}
	logging.Logf(types.LogInfo, "Successfully fetched server info - ID: %s, Name: %s", serverInfo.Id, serverInfo.ServerName)
	serverCache.Set(serverInfo)
	logging.Logf(types.LogDebug, "Server info cached at: %v", serverCache.Timestamp)
}

// startHTTPServer starts the HTTP server for the dashboard
func startHTTPServer(serverCache *types.ServerInfoCache, cfg *types.Config, requestStats *types.RequestStats, ipBlacklist *types.IPBlacklist) *http.Server {
	httpServer := &http.Server{
		Addr: fmt.Sprintf(":%s", cfg.HTTPPort),
	}

	http.HandleFunc("/health", web.HealthCheckHandler)
	http.HandleFunc("/", web.DashboardHandler(serverCache, cfg.ServerURL, cfg.ProxyURL, cfg.ProxyURLv6, requestStats, ipBlacklist, logging.LogBuffer, types.Version))
	http.HandleFunc("/static/", web.StaticFileHandler)
	http.HandleFunc("/favicon.ico", web.FaviconHandler)

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

// startListener starts the UDP listener goroutine. It receives IPv4 discovery
// requests and emits the primary response plus, when PROXY_URL_IPV6 is set,
// a second response carrying the v6 URL.
func startListener(ctx context.Context, conn *net.UDPConn, cfg *types.Config, serverCache *types.ServerInfoCache, ipBlacklist *types.IPBlacklist, requestStats *types.RequestStats, hookConfig *hooks.HookConfig) {
	logging.Logf(types.LogDebug, "Starting listener goroutine for %s", conn.LocalAddr())
	go discovery.ListenLoop(ctx, conn,
		cfg.ServerURL, cfg.ProxyURL, cfg.ProxyURLv6,
		serverCache,
		ipBlacklist, requestStats, hookConfig)
}

// gracefulShutdown performs graceful shutdown of all services
func gracefulShutdown(cancel context.CancelFunc, httpServer *http.Server, conn *net.UDPConn) {
	// Cancel context to signal goroutines to stop
	cancel()

	// Shutdown HTTP server
	logging.Logln(types.LogInfo, "Shutting down HTTP server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logging.Logf(types.LogWarn, "HTTP server shutdown error: %v", err)
	}

	// Close UDP connection
	logging.Logf(types.LogInfo, "Closing UDP listener: %s", conn.LocalAddr())
	conn.Close()

	// Give goroutines a moment to finish
	time.Sleep(100 * time.Millisecond)
}
