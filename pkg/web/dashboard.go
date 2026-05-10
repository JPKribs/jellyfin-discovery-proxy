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

//go:embed assets/favicon.ico
var faviconICO []byte

// StartTime holds the application start time for uptime calculation
var StartTime time.Time

// HealthCheckHandler handles health check requests
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// DashboardHandler returns an HTTP handler for the dashboard
func DashboardHandler(serverCache *types.ServerInfoCache, serverURL, proxyURL, proxyURLv6 string, stats *types.RequestStats, blacklist *types.IPBlacklist, logBuffer *types.LogBuffer, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lastReqTime, lastReqIP, totalReqs := stats.GetStats()

		serverInfo := serverCache.Get()
		cachedID := "N/A"
		cachedName := "N/A"
		cacheAge := "N/A"

		if serverInfo != nil {
			cachedID = serverInfo.Id
			cachedName = serverInfo.ServerName
			cacheAge = time.Since(serverCache.Timestamp).Round(time.Second).String()
		}

		lastReqTimeStr := "Never"
		if !lastReqTime.IsZero() {
			lastReqTimeStr = lastReqTime.Format("2006-01-02 15:04:05")
		}

		uptime := time.Since(StartTime).Round(time.Second).String()
		logs := logBuffer.GetAll()

		proxyURLv6Display := proxyURLv6
		if proxyURLv6Display == "" {
			proxyURLv6Display = "(not set)"
		}

		data := types.DashboardData{
			Version:          version,
			ServerURL:        serverURL,
			ProxyURL:         proxyURL,
			ProxyURLv6:       proxyURLv6Display,
			LastRequestTime:  lastReqTimeStr,
			LastRequestIP:    lastReqIP,
			TotalRequests:    totalReqs,
			CachedServerID:   cachedID,
			CachedServerName: cachedName,
			CacheAge:         cacheAge,
			BlacklistedIPs:   blacklist.Count(),
			Logs:             logs,
			Uptime:           uptime,
		}

		t := template.Must(template.New("dashboard").Parse(dashboardHTML))
		w.Header().Set("Content-Type", "text/html")
		t.Execute(w, data)
	}
}

// StaticFileHandler serves static files (CSS, JS)
func StaticFileHandler(w http.ResponseWriter, r *http.Request) {
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

// FaviconHandler serves the favicon
func FaviconHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Write(faviconICO)
}
