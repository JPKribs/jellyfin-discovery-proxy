# Build stage
FROM golang:1.24-alpine AS builder

# Copy Go module files to working directory and download dependencies
WORKDIR /app
COPY go.mod ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o jellyfin-discovery-proxy

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/jellyfin-discovery-proxy .

# Expose UDP port 7359
EXPOSE 7359/udp

# Run the application
CMD ["./jellyfin-discovery-proxy"]