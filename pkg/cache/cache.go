package cache

import (
	"os"
	"strconv"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/logging"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// New creates a new empty ServerInfoCache instance with specified cache duration
func New(cacheDuration time.Duration) *types.ServerInfoCache {
	return &types.ServerInfoCache{
		Info:      nil,
		Timestamp: time.Time{},
		Duration:  cacheDuration,
	}
}

// GetDuration parses CACHE_DURATION environment variable and returns appropriate duration
func GetDuration() time.Duration {
	cacheDurationStr := os.Getenv("CACHE_DURATION")
	if cacheDurationStr == "" {
		logging.Logln(types.LogInfo, "CACHE_DURATION environment variable not set, using default 24 hours")
		return 24 * time.Hour
	}

	// If explicitly set to 0, cache until restart
	if cacheDurationStr == "0" {
		logging.Logln(types.LogInfo, "CACHE_DURATION set to 0, caching until restart")
		return 0
	}

	// Parse the hours value
	hours, err := strconv.Atoi(cacheDurationStr)
	if err != nil {
		logging.Logf(types.LogWarn, "Invalid CACHE_DURATION value: %s, using default 24 hours", cacheDurationStr)
		return 24 * time.Hour
	}

	logging.Logf(types.LogInfo, "CACHE_DURATION set to %d hours", hours)
	return time.Duration(hours) * time.Hour
}
