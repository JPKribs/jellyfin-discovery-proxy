#!/bin/bash
# build.sh - Cross-platform build script for Jellyfin Discovery Proxy
# This script builds binaries for Windows, macOS, and Linux (including OpenWRT)

set -e

# Application name and version
APP_NAME="jellyfin-discovery-proxy"
VERSION="1.0.0"
BUILD_DIR="./build"

# Print colored text
print_color() {
  if [ -t 1 ]; then
    echo -e "\033[1;34m$1\033[0m"
  else
    echo "$1"
  fi
}

# Clean build directory
clean() {
  print_color "==> Cleaning build directory"
  rm -rf "$BUILD_DIR"
  mkdir -p "$BUILD_DIR"
}

# Build for a specific platform
build() {
  local GOOS=$1
  local GOARCH=$2
  local SUFFIX=$3
  local GOMIPS=$4
  local CGO=$5
  local OUTPUT=""
  
  # Set default values if not provided
  CGO=${CGO:-0}
  
  # Set output file name based on OS
  if [ "$GOOS" = "windows" ]; then
    OUTPUT="${BUILD_DIR}/${APP_NAME}_${VERSION}_${SUFFIX}.exe"
  else
    OUTPUT="${BUILD_DIR}/${APP_NAME}_${VERSION}_${SUFFIX}"
  fi
  
  print_color "==> Building for $GOOS/$GOARCH${GOMIPS:+ (GOMIPS=$GOMIPS)}${CGO:+ (CGO_ENABLED=$CGO)}"
  
  # Prepare build flags for optimizing binary size
  local BUILD_FLAGS="-trimpath -ldflags=\"-s -w\""
  
  # Set environment variables and build
  if [ -n "$GOMIPS" ]; then
    env GOOS=$GOOS GOARCH=$GOARCH GOMIPS=$GOMIPS CGO_ENABLED=$CGO go build -trimpath -ldflags="-s -w" -o "$OUTPUT" main.go
  else
    env GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=$CGO go build -trimpath -ldflags="-s -w" -o "$OUTPUT" main.go
  fi
  
  # Create zip archive for the binary
  if [ -f "$OUTPUT" ]; then
    if [ "$GOOS" = "windows" ]; then
      (cd "$BUILD_DIR" && zip -q "${APP_NAME}_${VERSION}_${SUFFIX}.zip" "$(basename "$OUTPUT")")
    else
      (cd "$BUILD_DIR" && tar -czf "${APP_NAME}_${VERSION}_${SUFFIX}.tar.gz" "$(basename "$OUTPUT")")
    fi
    
    # Print size information
    SIZE=$(du -h "$OUTPUT" | cut -f1)
    print_color "   Built: $OUTPUT ($SIZE)"
  else
    print_color "   Failed to build: $OUTPUT"
    return 1
  fi
}

# Build docker image
build_docker() {
  print_color "==> Building Docker image"
  docker build -t "${APP_NAME}:${VERSION}" .
  docker tag "${APP_NAME}:${VERSION}" "${APP_NAME}:latest"
  print_color "   Docker image built: ${APP_NAME}:${VERSION}"
}

# Main build function that builds for all platforms
build_all() {
  clean
  
  # Standard 64-bit platforms
  build "linux" "amd64" "linux-amd64" "" "0"
  build "windows" "amd64" "windows-amd64" "" "0"
  build "darwin" "amd64" "macos-amd64" "" "0"
  
  # Standard 32-bit platforms
  build "linux" "386" "linux-386" "" "0"
  build "windows" "386" "windows-386" "" "0"
  
  # ARM platforms
  build "linux" "arm64" "linux-arm64" "" "0"
  build "linux" "arm" "linux-armv7" "" "0"
  build "darwin" "arm64" "macos-arm64" "" "0"
  
  # OpenWRT/MIPS platforms
  build "linux" "mips" "linux-mips" "softfloat" "0"
  build "linux" "mipsle" "linux-mipsle" "softfloat" "0"
  
  # Create a checksum file for all builds
  print_color "==> Generating checksums"
  (cd "$BUILD_DIR" && sha256sum * > "${APP_NAME}_${VERSION}_checksums.txt")
  
  # Build Docker image
  build_docker
  
  print_color "==> Build Complete!"
  print_color "   All binaries and archives are in the $BUILD_DIR directory"
}

# Show help message
show_help() {
  echo "Usage: $0 [options]"
  echo ""
  echo "Options:"
  echo "  -h, --help       Show this help message"
  echo "  -c, --clean      Clean the build directory"
  echo "  -d, --docker     Build Docker image only"
  echo "  --linux          Build Linux binaries only"
  echo "  --windows        Build Windows binaries only"
  echo "  --mac            Build macOS binaries only"
  echo "  --openwrt        Build OpenWRT/MIPS binaries only"
  echo ""
  echo "Without any options, all platforms will be built."
}

# Parse command line arguments
case "$1" in
  "-h"|"--help")
    show_help
    exit 0
    ;;
  "-c"|"--clean")
    clean
    exit 0
    ;;
  "-d"|"--docker")
    build_docker
    exit 0
    ;;
  "--linux")
    clean
    build "linux" "amd64" "linux-amd64" "" "0"
    build "linux" "386" "linux-386" "" "0"
    build "linux" "arm64" "linux-arm64" "" "0"
    build "linux" "arm" "linux-armv7" "" "0"
    exit 0
    ;;
  "--windows")
    clean
    build "windows" "amd64" "windows-amd64" "" "0"
    build "windows" "386" "windows-386" "" "0"
    exit 0
    ;;
  "--mac")
    clean
    build "darwin" "amd64" "macos-amd64" "" "0"
    build "darwin" "arm64" "macos-arm64" "" "0"
    exit 0
    ;;
  "--openwrt")
    clean
    build "linux" "mips" "linux-mips" "softfloat" "0"
    build "linux" "mipsle" "linux-mipsle" "softfloat" "0"
    exit 0
    ;;
  *)
    build_all
    ;;
esac