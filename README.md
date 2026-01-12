# Jellyfin Discovery Proxy

## Overview

Jellyfin Discovery Proxy enables automatic server discovery for Jellyfin clients when UDP packets cannot reach your Jellyfin server. This is particularly useful when clients are connecting through VPN connections or across network boundaries where broadcast discovery typically fails.

## How It Works

Jellyfin uses a UDP discovery protocol on port 7359 where clients broadcast a "Who is JellyfinServer?" message to automatically find servers on the network. 

This proxy:
1. Runs on a device that can receive these UDP broadcasts
2. Listens for discovery requests on port 7359
3. Forwards the request to your actual Jellyfin server via HTTP
4. Returns the server information to clients in the expected format

This allows automatic discovery to work even in network configurations where UDP broadcasts can't reach your server.

![Jellyfin Discovery Proxy Diagram](JDP-Diagram.png)

## Project Structure

```
jellyfin-discovery-proxy/
├── Makefile                # Build system for cross-platform compilation
├── project.conf            # Project metadata configuration
├── main.go                 # Main application code
├── Dockerfile              # Docker container definition
├── README.md               # This documentation file
├── LICENSE                 # MIT License
├── scripts/                # Helper scripts
│   └── docker-build.sh     # Docker multi-platform build helper
└── build/                  # Output directory for compiled binaries (created during build)
```

## Common Use Cases

### VPN Client Connections

When clients connect to your network through any VPN solution, they often can't discover Jellyfin servers through normal means. The proxy creates a "local server" entry on Jellyfin clients as if the server was directly on their network.

### Docker and Container Environments

In containerized environments, network isolation can prevent discovery requests from reaching your Jellyfin container.

### Separated Network Segments

Works across VLANs, subnets, or other network boundaries where broadcast traffic is filtered or blocked.

### Remote Access Scenarios

Enables discovery for remote Jellyfin servers as if they were on the local network.

## Running with Docker

The easiest way to run this application is with Docker:

```bash
# Pull and run with environment variables
docker run -d \
  --name jellyfin-discovery-proxy \
  --network=host \
  -p 8080:8080 \
  -e JELLYFIN_SERVER_URL=http://your-jellyfin-server.com:8096 \
  -e PROXY_URL=http://ip-or-friendly-name-of-device.local \
  -e CACHE_DURATION=12 \
  -e LOG_LEVEL=info \
  jpkribs/jellyfin-discovery-proxy

# Or with debug logging and IP blacklist
docker run -d \
  --name jellyfin-discovery-proxy \
  --network=host \
  -p 8080:8080 \
  -e JELLYFIN_SERVER_URL=http://your-jellyfin-server.com:8096 \
  -e LOG_LEVEL=debug \
  -e BLACKLIST=192.168.0.100,192.168.1.0/24 \
  jpkribs/jellyfin-discovery-proxy

# Or with separate IPv4 and IPv6 server URLs
docker run -d \
  --name jellyfin-discovery-proxy \
  --network=host \
  -p 8080:8080 \
  -e JELLYFIN_SERVER_URL_IPV4=http://192.168.1.100:8096 \
  -e JELLYFIN_SERVER_URL_IPV6=http://[2001:db8::1]:8096 \
  -e LOG_LEVEL=info \
  jpkribs/jellyfin-discovery-proxy
```

### Docker Compose Example

Create a `docker-compose.yml` file with the following contents:

```yaml
version: '3'

services:
  jellyfin-discovery-proxy:
    image: jpkribs/jellyfin-discovery-proxy:latest
    container_name: jellyfin-discovery-proxy
    network_mode: host # Required: Bridged/VLAN networks typically do not receive discovery broadcasts
    restart: unless-stopped
    ports:
      - "8080:8080" # Web dashboard and health check
    environment:
      # Legacy mode - use same URL for both IPv4 and IPv6
      - JELLYFIN_SERVER_URL=http://your-jellyfin-server.com:8096
      - PROXY_URL=http://ip-or-friendly-name-of-device.local # Optional: use a local name different than the server url

      # Dual-stack mode - separate URLs for IPv4 and IPv6 (optional, overrides legacy mode)
      # - JELLYFIN_SERVER_URL_IPV4=http://192.168.1.100:8096
      # - JELLYFIN_SERVER_URL_IPV6=http://[2001:db8::1]:8096
      # - PROXY_URL_IPV4=http://192.168.1.100:8096
      # - PROXY_URL_IPV6=http://[2001:db8::1]:8096

      - CACHE_DURATION=12 # Optional: cache server info for 12 hours
      - LOG_LEVEL=info # Optional: set log level (debug, info, warn, error)
      - LOG_BUFFER_SIZE=1024 # Optional: number of log lines to keep in memory
      - BLACKLIST= # Optional: comma-separated list of IPs/subnets to block (e.g., 192.168.0.1,192.168.1.0/24)
      - NETWORK_INTERFACE= # Optional: bind to specific interface (e.g., eth0)
```

Then run with Docker Compose:
```bash
docker-compose up -d
```

## Building from Source

If you prefer to build from source:

```bash
# Clone repository
git clone https://github.com/jpkribs/jellyfin-discovery-proxy.git
cd jellyfin-discovery-proxy

# Build
go build -o jellyfin-discovery-proxy

# Run with default log level (info)
PROXY_URL=http://ip-or-friendly-name-of-device.local JELLYFIN_SERVER_URL=https://your-jellyfin-server.com:8096 CACHE_DURATION=6 ./jellyfin-discovery-proxy

# Or with debug logging
PROXY_URL=http://ip-or-friendly-name-of-device.local JELLYFIN_SERVER_URL=https://your-jellyfin-server.com:8096 ./jellyfin-discovery-proxy -log-level debug
```

## Using the Makefile

The project includes a Makefile that can compile the application for various platforms:

```bash
# Build for all platforms (Windows, macOS, Linux, and OpenWRT)
make

# Build only for specific platforms
make linux     # Build only Linux binaries
make windows   # Build only Windows binaries
make mac       # Build only macOS binaries (mac alias works)
make macos     # Build only macOS binaries (macos alias works)
make openwrt   # Build only OpenWRT/MIPS binaries

# Build Docker image only
make docker

# Clean the build directory
make clean

# View all available options
make help
```

After running the build commands, you'll find the binaries and archives in the `build` directory.

## Configuration File

The project uses a `project.conf` file to store metadata:

```makefile
# Application metadata
APP_NAME := jellyfin-discovery-proxy
VERSION := 1.3.0
OWNER := jpkribs

# Build directory
BUILD_DIR := ./build
```

This configuration is used by the Makefile and Docker build process. When updating the project version or changing the owner, just modify this file.

## Cross-Platform Compatibility

This application is designed to run on multiple platforms:

### OpenWRT - Untested

The Go implementation can be cross-compiled for OpenWRT routers (including MIPS architectures) by using:

```bash
make openwrt
```

The resulting binary is small enough to run on most OpenWRT devices with limited storage and memory.

### Windows, macOS, and Linux

Cross-compilation for major desktop platforms is simple with the Makefile:

```bash
# For Windows
make windows

# For macOS
make mac  # or make macos (both work)

# For Linux
make linux
```

## Runtime Configuration

The application is configured using environment variables and command-line flags:

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `JELLYFIN_SERVER_URL` | The URL to your Jellyfin server for API calls (legacy mode, used for both IPv4 and IPv6) | `http://localhost:8096` |
| `JELLYFIN_SERVER_URL_IPV4` | The URL to your Jellyfin server for IPv4 API calls | Uses `JELLYFIN_SERVER_URL` if not set |
| `JELLYFIN_SERVER_URL_IPV6` | The URL to your Jellyfin server for IPv6 API calls | Uses `JELLYFIN_SERVER_URL` if not set |
| `PROXY_URL` | Optional URL to use in discovery responses (legacy mode, used for both IPv4 and IPv6) | Uses `JELLYFIN_SERVER_URL` if not set |
| `PROXY_URL_IPV4` | Optional URL to use in IPv4 discovery responses | Uses `JELLYFIN_SERVER_URL_IPV4` if not set |
| `PROXY_URL_IPV6` | Optional URL to use in IPv6 discovery responses | Uses `JELLYFIN_SERVER_URL_IPV6` if not set |
| `CACHE_DURATION` | Number of hours to cache server information | `24` |
| `LOG_LEVEL` | Logging verbosity level (`debug`, `info`, `warn`, `error`) | `info` |
| `LOG_BUFFER_SIZE` | Number of log lines to retain in memory for dashboard | `1024` |
| `BLACKLIST` | Comma-separated list of IP addresses/subnets to ignore (e.g., `192.168.0.1,192.168.1.0/24`) | None |
| `NETWORK_INTERFACE` | Specific network interface to bind to (e.g., `eth0`, `wlan0`) | All interfaces |

### Command-Line Flags

| Flag | Description | Options | Default |
|------|-------------|---------|---------|
| `-log-level` | Set the logging verbosity level | `debug`, `info`, `warn`, `error` | `info` |

**Log Level Details:**
- `debug` - Verbose debugging information including hex dumps, goroutine tracking, and detailed internal state
- `info` - Standard operational messages about discovery requests and responses
- `warn` - Warning messages for non-critical issues
- `error` - Error messages for critical failures only

**Example Usage:**
```bash
# Run with debug logging
./jellyfin-discovery-proxy -log-level debug

# Run with error logging only
JELLYFIN_SERVER_URL=http://server:8096 ./jellyfin-discovery-proxy -log-level error
```

### Cache Duration Options

The `CACHE_DURATION` environment variable controls how long server information is cached:

- Default: `24` (caches for 24 hours)
- Any positive number: Caches for that many hours
- `0`: Caches until the application restarts (useful for stable environments)
- Not set: Uses the default of 24 hours

Adjust this value based on your specific needs:
- Lower values (1-6 hours) for frequently changing server configurations
- Higher values (24+ hours) for stable environments
- Set to `0` to minimize API calls in unchanging environments

### IPv4/IPv6 Dual-Stack Configuration

The proxy supports separate URLs for IPv4 and IPv6 connections:

**Legacy Mode (Backward Compatible):**
```bash
# Use the same URL for both IPv4 and IPv6
JELLYFIN_SERVER_URL=http://your-server:8096
PROXY_URL=http://your-proxy:8096
```

**Dual-Stack Mode:**
```bash
# Specify different URLs for IPv4 and IPv6
JELLYFIN_SERVER_URL_IPV4=http://192.168.1.100:8096
JELLYFIN_SERVER_URL_IPV6=http://[2001:db8::1]:8096
PROXY_URL_IPV4=http://192.168.1.100:8096
PROXY_URL_IPV6=http://[2001:db8::1]:8096
```

**Fallback Behavior:**
- If only one IP version URL is set, it will be used for both IPv4 and IPv6
- If neither IPv4 nor IPv6 specific URLs are set, falls back to legacy `JELLYFIN_SERVER_URL`
- Proxy URLs default to their corresponding server URLs if not specified

**Address Selection Logic:**

1. IPv4 clients receive responses using `PROXY_URL_IPV4` (or `JELLYFIN_SERVER_URL_IPV4`)
2. IPv6 clients receive responses using `PROXY_URL_IPV6` (or `JELLYFIN_SERVER_URL_IPV6`)
3. This allows different network paths for IPv4 and IPv6 clients

This configuration is particularly useful when:
- Your Jellyfin server is dual-stacked with different addresses for IPv4 and IPv6
- You want to optimize routing for each IP version
- IPv4 and IPv6 clients need to reach the server through different network paths

## Deployment Recommendations

### Home Router or Network Device

Install on a device that can receive broadcast traffic from all client devices. A router running OpenWRT, a Raspberry Pi, or a NAS device works well.

### Cloud or Remote Server

When your Jellyfin instance is hosted remotely, deploy the proxy locally on your network to make remote servers appear as local ones.

### Multi-Network Setups

For environments with multiple network segments, consider deploying one proxy instance per segment where clients need discovery.

## Features

- **IPv4/IPv6 Dual-Stack Support**: Separate URLs and caches for IPv4 and IPv6 connections with automatic fallback
- **Configurable Caching**: Customize how long server information is cached (per IP version)
- **Smart Fallback**: Won't respond if the Jellyfin server is unreachable
- **Lightweight**: Minimal resource usage makes it suitable for small devices
- **Cross-Platform**: Runs on virtually any operating system
- **Dual URL Support**: Separate internal server URL and advertised client URL (per IP version)
- **Avahi Support**: Outputs both Avahi service.local and direct IP for usage on non-Avahi eligible devices
- **Web Dashboard**: Built-in web interface on port 8080 showing server info, request statistics, and live logs with manual refresh
- **Health Check Endpoint**: HTTP health check on port 8080 for monitoring systems
- **IP/Subnet Blacklist**: Block specific IP addresses or entire subnets using CIDR notation from receiving discovery responses
- **Network Interface Selection**: Bind to specific network interfaces for multi-homed systems
- **In-Memory Log Buffer**: Configurable log retention for dashboard viewing

## Web Dashboard

The proxy includes a built-in web dashboard available at `http://localhost:8080` (or the IP of the device running the proxy). The dashboard provides:

- Real-time server information for both IPv4 and IPv6 (name, ID, cache age)
- Separate configuration display for IPv4 and IPv6 URLs
- Request statistics (last request time, last IP, total requests)
- Configuration overview including blacklisted IPs/subnets
- Live log viewing with manual refresh button and timer
- Uptime tracking

### Health Check

A health check endpoint is available at `http://localhost:8080/health` that returns HTTP 200 with "OK". This is useful for:
- Docker health checks
- Kubernetes liveness/readiness probes
- External monitoring systems

## Advanced Configuration

### IP Blacklist

Block specific IP addresses or entire subnets from receiving discovery responses:

```bash
# Block single IP
BLACKLIST=192.168.0.100

# Block multiple IPs
BLACKLIST=192.168.0.100,192.168.0.101,10.0.0.50

# Block entire subnets using CIDR notation
BLACKLIST=192.168.1.0/24,10.0.0.0/8

# Mix individual IPs and subnets
BLACKLIST=192.168.0.100,192.168.1.0/24,10.0.0.5
```

Blacklisted IPs and subnets will receive no response to their discovery requests, and the requests will be logged as warnings. CIDR notation allows you to block entire network ranges efficiently.

### Network Interface Selection

Bind to a specific network interface instead of all interfaces:

```bash
# Bind to specific interface
NETWORK_INTERFACE=eth0

# Or for wireless
NETWORK_INTERFACE=wlan0
```

This is useful for:
- Multi-homed systems with multiple network interfaces
- Limiting discovery to specific VLANs or subnets
- Security hardening by restricting which networks can discover the server

### Log Buffer Configuration

Control how many log lines are kept in memory for the dashboard:

```bash
# Keep last 500 lines
LOG_BUFFER_SIZE=500

# Keep last 2000 lines
LOG_BUFFER_SIZE=2000
```

Note: This only affects the dashboard view. All logs are still written to stdout/stderr for Docker logging.

## Troubleshooting

If you're having issues:

1. Check that UDP port 7359 is open on your firewall
2. Check that TCP port 8080 is accessible for the dashboard
3. Verify your Jellyfin server is accessible at the URL specified
4. Check the logs with `docker logs jellyfin-discovery-proxy` or view them in the web dashboard
5. Make sure the `/System/Info/Public` endpoint is accessible on your Jellyfin server
6. Use the web dashboard at `http://<proxy-ip>:8080` to view real-time statistics and logs

## License

This project is licensed under the [MIT License](https://github.com/JPKribs/jellyfin-discovery-proxy/blob/main/LICENSE).