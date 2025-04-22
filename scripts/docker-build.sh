#!/bin/bash
# Simple docker-build.sh - Docker build helper script for Jellyfin Discovery Proxy
# This script builds and pushes multi-platform Docker images

# Exit on error
set -e

# Check for arguments
if [ "$#" -lt 3 ]; then
  echo "Usage: $0 <app_name> <version> <owner>"
  exit 1
fi

# Get parameters
APP_NAME="$1"
VERSION="$2"
OWNER="$3"

echo "==> Checking Docker availability"
if ! command -v docker &>/dev/null; then
  echo "Error: Docker is not installed or not in PATH."
  exit 1
fi

echo "==> Setting up Docker Buildx"
if ! docker buildx version &>/dev/null; then
  echo "Error: Docker Buildx plugin is not available."
  echo "Please ensure you have Docker >= 19.03 and buildx plugin is installed."
  exit 1
fi

# Create or use builder
if ! docker buildx inspect --builder custom-builder &>/dev/null; then
  echo "   Creating new Buildx builder instance"
  docker buildx create --name custom-builder --use
else
  echo "   Using existing Buildx builder instance"
  docker buildx use custom-builder
fi

# Bootstrap builder
if ! docker buildx inspect --bootstrap &>/dev/null; then
  echo "   Bootstrapping Buildx builder"
  docker buildx inspect --bootstrap
fi

echo "==> Building Docker images"

# Check for Dockerfile
if [ ! -f "Dockerfile" ]; then
  echo "Error: Dockerfile not found in current directory"
  exit 1
fi

# Multi-platform build (amd64 and arm64)
echo "   Building Docker image for amd64 and arm64"
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag "${OWNER}/${APP_NAME}:${VERSION}" \
  --tag "${OWNER}/${APP_NAME}:latest" \
  --build-arg VERSION="${VERSION}" \
  --build-arg BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
  --build-arg VCS_REF="$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')" \
  --push \
  .

echo "   Docker images built and pushed: ${OWNER}/${APP_NAME}:${VERSION} and ${OWNER}/${APP_NAME}:latest"