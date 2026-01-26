# Build stage - GO_VERSION arg allows override, defaults to 1.21
ARG GO_VERSION=1.21
FROM golang:${GO_VERSION}-alpine AS builder

# Copy Go module files to working directory and download dependencies
WORKDIR /app
COPY go.mod ./
RUN go mod download

# Copy source code and project config
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
COPY project.conf ./

# Build the application with version from project.conf
RUN source project.conf && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X 'jellyfin-discovery-proxy/pkg/types.Version=${VERSION}'" \
    -o jellyfin-discovery-proxy ./cmd/jellyfin-discovery-proxy

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/jellyfin-discovery-proxy .

# Expose UDP port 7359 and HTTP port 8080
EXPOSE 7359/udp
EXPOSE 8080/tcp

# Environment variables with defaults
ENV JELLYFIN_SERVER_URL="http://localhost:8096"
ENV PROXY_URL=""

# Run the application
CMD ["./jellyfin-discovery-proxy"]