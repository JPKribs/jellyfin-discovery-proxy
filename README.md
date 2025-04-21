# Jellyfin Discovery Proxy

## Overview

This lightweight application solves a specific but common problem encountered when running a Jellyfin media server behind a VPN tunnel like WireGuard: the inability for clients on your local network to automatically discover your Jellyfin server.

## The Problem

Jellyfin uses a UDP-based discovery protocol that operates on port 7359. When a client (such as a TV app, mobile app, or desktop client) first starts, it sends a UDP broadcast message "Who is JellyfinServer?" to port 7359 across the local network. Jellyfin servers listening on that port respond with a JSON packet containing their connection details, allowing clients to automatically detect and list available media servers without manual configuration.

However, this discovery mechanism breaks when your Jellyfin server is:

1. **Behind a VPN tunnel like WireGuard**: UDP broadcast packets don't pass through the tunnel
2. **Running in a Docker container with custom networking**: UDP broadcast might not reach the container
3. **On a different VLAN or subnet**: Broadcast domains might not reach the server
4. **Running on a remote/cloud server**: Local clients can't discover remote servers via UDP broadcast

In these scenarios, users have to manually enter their server details on every client device instead of enjoying the convenience of auto-discovery.

## The Solution

This discovery proxy acts as a bridge between your local network and your Jellyfin server by:

1. Running on your local network (or a device that can receive local UDP broadcasts)
2. Listening for Jellyfin discovery requests on UDP port 7359
3. When a request is received, it calls the public API of your actual Jellyfin server
4. Formats the response to match the Jellyfin discovery protocol format
5. Sends the response back to the client

The result: all your Jellyfin clients can automatically discover your server again, even though it's behind a VPN!

## Running with Docker

The easiest way to run this application is with Docker:

```bash
# Pull and run with environment variable
docker run -d \
  --name jellyfin-discovery-proxy \
  -p 7359:7359/udp \
  -e JELLYFIN_SERVER_URL=https://your-jellyfin-server.com:8096 \
  yourusername/jellyfin-discovery-proxy
```

Or with Docker Compose:

1. Edit the `docker-compose.yml` file to set your server URL:
   ```yaml
   environment:
     - JELLYFIN_SERVER_URL=https://your-jellyfin-server.com:8096
   ```

2. Run with Docker Compose:
   ```bash
   docker-compose up -d
   ```

## Building from Source

If you prefer to build from source:

```bash
# Clone repository
git clone https://github.com/yourusername/jellyfin-discovery-proxy.git
cd jellyfin-discovery-proxy

# Build
go build -o jellyfin-discovery-proxy

# Run
JELLYFIN_SERVER_URL=https://your-jellyfin-server.com:8096 ./jellyfin-discovery-proxy
```

## Cross-Platform Compatibility

This application is designed to run on multiple platforms:

### OpenWRT

The Go implementation can be cross-compiled for OpenWRT routers (including MIPS architectures) by using:

```bash
GOOS=linux GOARCH=mips GOMIPS=softfloat CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"
```

The resulting binary is small enough to run on most OpenWRT devices with limited storage and memory.

### Windows, macOS, and Linux

Cross-compilation for major desktop platforms is simple with Go:

```bash
# For Windows
GOOS=windows GOARCH=amd64 go build -o jellyfin-discovery-proxy.exe

# For macOS
GOOS=darwin GOARCH=amd64 go build -o jellyfin-discovery-proxy-mac

# For Linux
GOOS=linux GOARCH=amd64 go build -o jellyfin-discovery-proxy-linux
```

## Configuration

The application is configured using environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `JELLYFIN_SERVER_URL` | The URL to your Jellyfin server | `http://localhost:8096` |

## Practical Use Cases

### Home Network with WireGuard VPN

You have a Jellyfin server behind a WireGuard VPN for secure remote access, but local clients (smart TVs, mobile apps) can't discover it automatically.

**Solution**: Run this proxy on a device inside your local network (like a Raspberry Pi, NAS, or even your router if it supports OpenWRT).

### Docker Deployment with Complex Networking

Your Jellyfin server runs in a Docker container with custom networking that blocks UDP broadcasts from reaching it.

**Solution**: Run this proxy in a separate container with host networking to bridge discovery requests to your Jellyfin container.

### Multi-VLAN Network Segmentation

You have your Jellyfin server on a separate VLAN from your client devices for improved security or network organization.

**Solution**: Run the proxy on a device that can communicate across your VLANs or on each VLAN that contains client devices.

### Split Home/Cloud Setup

You run Jellyfin on a cloud server but want local clients to discover it as if it were on your home network.

**Solution**: Run the proxy on your local network to relay discovery requests to your cloud instance.

## Why This Matters

For Jellyfin users, automatic discovery is a key part of what makes the experience seamless. Having to manually enter server details on every device (especially TV apps with clunky remote controls) creates unnecessary friction.

This simple proxy preserves the plug-and-play experience of Jellyfin's auto-discovery while giving you the flexibility to deploy your server wherever and however you want.

## Caching & Performance

- Server information is cached for 24 hours to minimize API calls
- If the Jellyfin server becomes unreachable, the proxy won't respond to discovery requests
- Minimal resource usage makes it suitable for low-powered devices like OpenWRT routers

## Troubleshooting

If you're having issues:

1. Check that UDP port 7359 is open on your firewall
2. Verify your Jellyfin server is accessible at the URL specified
3. Check the logs with `docker logs jellyfin-discovery-proxy`
4. Make sure the `/System/Info/Public` endpoint is accessible on your Jellyfin server

## License

MIT