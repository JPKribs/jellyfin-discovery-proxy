package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/logging"
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// HookConfig holds webhook configuration.
type HookConfig struct {
	OnReceiveURL string
	OnReceiveCmd string
	OnSendURL    string
	OnSendCmd    string
}

// LoadHookConfig loads hook configuration from environment variables.
func LoadHookConfig() *HookConfig {
	return &HookConfig{
		OnReceiveURL: os.Getenv("HOOK_ON_RECEIVE_URL"),
		OnReceiveCmd: os.Getenv("HOOK_ON_RECEIVE_CMD"),
		OnSendURL:    os.Getenv("HOOK_ON_SEND_URL"),
		OnSendCmd:    os.Getenv("HOOK_ON_SEND_CMD"),
	}
}

// OnReceivePayload contains data sent to onReceive hooks.
type OnReceivePayload struct {
	Timestamp   time.Time `json:"timestamp"`
	ClientIP    string    `json:"client_ip"`
	ClientPort  int       `json:"client_port"`
	Message     string    `json:"message"`
	LocalSocket string    `json:"local_socket"`
}

// OnSendPayload contains data sent to onSend hooks.
type OnSendPayload struct {
	Timestamp     time.Time `json:"timestamp"`
	ClientIP      string    `json:"client_ip"`
	ClientPort    int       `json:"client_port"`
	ServerID      string    `json:"server_id"`
	ServerName    string    `json:"server_name"`
	AddressURL    string    `json:"address_url"`
	ResponseBytes int       `json:"response_bytes"`
}

// ExecuteOnReceive executes configured onReceive hooks.
func (hc *HookConfig) ExecuteOnReceive(payload OnReceivePayload) error {
	if hc.OnReceiveURL == "" && hc.OnReceiveCmd == "" {
		logging.Logf(types.LogDebug, "No onReceive hook configured, skipping")
		return nil
	}

	logging.Logf(types.LogDebug, "Executing onReceive hook for client %s", payload.ClientIP)

	if hc.OnReceiveURL != "" {
		if err := executeWebhook(hc.OnReceiveURL, payload, "onReceive"); err != nil {
			logging.Logf(types.LogWarn, "onReceive webhook failed: %v", err)
			return err
		}
	}

	if hc.OnReceiveCmd != "" {
		if err := executeCommand(hc.OnReceiveCmd, payload, "onReceive"); err != nil {
			logging.Logf(types.LogWarn, "onReceive command failed: %v", err)
			return err
		}
	}

	logging.Logf(types.LogInfo, "Successfully executed onReceive hook for %s", payload.ClientIP)
	return nil
}

// ExecuteOnSend executes configured onSend hooks.
func (hc *HookConfig) ExecuteOnSend(payload OnSendPayload) error {
	if hc.OnSendURL == "" && hc.OnSendCmd == "" {
		logging.Logf(types.LogDebug, "No onSend hook configured, skipping")
		return nil
	}

	logging.Logf(types.LogDebug, "Executing onSend hook for client %s", payload.ClientIP)

	if hc.OnSendURL != "" {
		if err := executeWebhook(hc.OnSendURL, payload, "onSend"); err != nil {
			logging.Logf(types.LogWarn, "onSend webhook failed: %v", err)
			return err
		}
	}

	if hc.OnSendCmd != "" {
		if err := executeCommand(hc.OnSendCmd, payload, "onSend"); err != nil {
			logging.Logf(types.LogWarn, "onSend command failed: %v", err)
			return err
		}
	}

	logging.Logf(types.LogInfo, "Successfully executed onSend hook for %s", payload.ClientIP)
	return nil
}

// executeWebhook sends a POST request with JSON payload to the webhook URL.
func executeWebhook(url string, payload interface{}, hookName string) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	logging.Logf(types.LogDebug, "Sending %s webhook to %s with payload: %s", hookName, url, string(jsonData))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "jellyfin-discovery-proxy/"+types.Version)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	logging.Logf(types.LogDebug, "%s webhook responded with status: %d", hookName, resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}

// executeCommand executes a shell command with JSON payload passed via stdin.
func executeCommand(command string, payload interface{}, hookName string) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	logging.Logf(types.LogDebug, "Executing %s command: %s", hookName, command)

	cmd := exec.Command("bash", "-c", command)
	cmd.Stdin = strings.NewReader(string(jsonData))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		logging.Logf(types.LogError, "%s command failed: %v, stderr: %s", hookName, err, stderr.String())
		return fmt.Errorf("command execution failed: %w", err)
	}

	if stdout.Len() > 0 {
		logging.Logf(types.LogDebug, "%s command stdout: %s", hookName, stdout.String())
	}
	if stderr.Len() > 0 {
		logging.Logf(types.LogDebug, "%s command stderr: %s", hookName, stderr.String())
	}

	logging.Logf(types.LogInfo, "%s command executed successfully", hookName)
	return nil
}
