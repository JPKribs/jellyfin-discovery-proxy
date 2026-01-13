package logging

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// Global log level configuration
var CurrentLog types.Log = types.LogInfo

// Global log buffer
var LogBuffer *types.LogBuffer

// NewLogBuffer creates a new log buffer with specified max size
func NewLogBuffer(maxSize int) *types.LogBuffer {
	return &types.LogBuffer{
		Messages: make([]string, 0, maxSize),
		MaxSize:  maxSize,
	}
}

// SetLog parses and sets the global log level from a string
func SetLog(level string) {
	switch strings.ToLower(level) {
	case "debug":
		CurrentLog = types.LogDebug
	case "info":
		CurrentLog = types.LogInfo
	case "warn", "warning":
		CurrentLog = types.LogWarn
	case "error":
		CurrentLog = types.LogError
	default:
		CurrentLog = types.LogInfo
		Logln(types.LogWarn, fmt.Sprintf("Unknown log level '%s', defaulting to 'info'", level))
	}
}

// shouldLog determines if a message at the given level should be logged
func shouldLog(level types.Log) bool {
	return level >= CurrentLog
}

// Logf - Custom logging function with level prefix and timestamp
func Logf(level types.Log, format string, v ...interface{}) {
	if !shouldLog(level) {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	prefix := fmt.Sprintf("[%s] %s | ", timestamp, level.String())
	formattedMsg := fmt.Sprintf(format, v...)
	fullMsg := prefix + formattedMsg
	log.Print(fullMsg)

	// Add to buffer if available
	if LogBuffer != nil {
		LogBuffer.Add(fullMsg)
	}
}

// Logln - Custom logging function with level prefix for simple messages
func Logln(level types.Log, message string) {
	if !shouldLog(level) {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	prefix := fmt.Sprintf("[%s] %s | ", timestamp, level.String())
	fullMsg := prefix + message
	log.Println(fullMsg)

	// Add to buffer if available
	if LogBuffer != nil {
		LogBuffer.Add(fullMsg)
	}
}

// GetLogBufferSize parses the LOG_BUFFER_SIZE environment variable
func GetLogBufferSize() int {
	bufferSizeStr := os.Getenv("LOG_BUFFER_SIZE")
	if bufferSizeStr == "" {
		return 100 // Default buffer size
	}

	var bufferSize int
	_, err := fmt.Sscanf(bufferSizeStr, "%d", &bufferSize)
	if err != nil || bufferSize <= 0 {
		Logf(types.LogWarn, "Invalid LOG_BUFFER_SIZE value: %s, using default 100", bufferSizeStr)
		return 100
	}

	return bufferSize
}
