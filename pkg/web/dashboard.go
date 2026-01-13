package web

import (
	_ "embed"
	"html/template"
	"net/http"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

//go:embed assets/dashboard.html
var dashboardHTML string

//go:embed assets/style.css
var styleCSS string

//go:embed assets/script.js
var scriptJS string

// StartTime holds the application start time for uptime calculation
var StartTime time.Time

// HealthCheckHandler handles health check requests
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// DashboardHandler returns an HTTP handler for the dashboard
func DashboardHandler(cacheV4, cacheV6 *types.ServerInfoCache, serverURLv4, serverURLv6, proxyURLv4, proxyURLv6 string, stats *types.RequestStats, blacklist *types.IPBlacklist, logBuffer *types.LogBuffer, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get stats
		lastReqTime, lastReqIP, totalReqs := stats.GetStats()

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
		uptime := time.Since(StartTime).Round(time.Second).String()

		// Get logs
		logs := logBuffer.GetAll()

		data := types.DashboardData{
			Version:            version,
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
			BlacklistedIPs:     blacklist.Count(),
			Logs:               logs,
			Uptime:             uptime,
		}

		t := template.Must(template.New("dashboard").Parse(dashboardHTML))
		w.Header().Set("Content-Type", "text/html")
		t.Execute(w, data)
	}
}

// StaticFileHandler serves static files (CSS, JS)
func StaticFileHandler(w http.ResponseWriter, r *http.Request) {
	// Map of file paths to content
	files := map[string]struct {
		content     string
		contentType string
	}{
		"/static/style.css": {
			content:     styleCSS,
			contentType: "text/css",
		},
		"/static/script.js": {
			content:     scriptJS,
			contentType: "application/javascript",
		},
	}

	file, ok := files[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", file.contentType)
	w.Write([]byte(file.content))
}
