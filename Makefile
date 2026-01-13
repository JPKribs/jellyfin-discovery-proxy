# Jellyfin Discovery Proxy Makefile
# Provides cross-platform build capabilities for the Jellyfin Discovery Proxy

# Include project configuration
-include project.conf

# Go build flags
LDFLAGS := -s -w
TRIMPATH := -trimpath

# Default goal
.PHONY: all
all: clean build-all

# Clean build directory
.PHONY: clean
clean:
	@echo "==> Cleaning build directory"
	@rm -rf $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)

# Build all platforms
.PHONY: build-all
build-all: linux windows mac openwrt checksums
	@echo "==> Build complete!"
	@echo "   All binaries and archives are in the $(BUILD_DIR) directory"
	@echo "   Docker images are built via GitHub Actions (.github/workflows/docker.yml)"

# Linux builds
.PHONY: linux
linux: linux-amd64 linux-386 linux-arm64 linux-armv7

.PHONY: linux-amd64
linux-amd64:
	@echo "==> Building for linux/amd64"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-amd64 ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && tar -czf $(APP_NAME)_$(VERSION)_linux-amd64.tar.gz $(APP_NAME)_$(VERSION)_linux-amd64
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-amd64 ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-amd64 ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-amd64 | cut -f1; else echo "N/A"; fi))"

.PHONY: linux-386
linux-386:
	@echo "==> Building for linux/386"
	@CGO_ENABLED=0 GOOS=linux GOARCH=386 go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-386 ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && tar -czf $(APP_NAME)_$(VERSION)_linux-386.tar.gz $(APP_NAME)_$(VERSION)_linux-386
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-386 ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-386 ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-386 | cut -f1; else echo "N/A"; fi))"

.PHONY: linux-arm64
linux-arm64:
	@echo "==> Building for linux/arm64"
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-arm64 ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && tar -czf $(APP_NAME)_$(VERSION)_linux-arm64.tar.gz $(APP_NAME)_$(VERSION)_linux-arm64
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-arm64 ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-arm64 ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-arm64 | cut -f1; else echo "N/A"; fi))"

.PHONY: linux-armv7
linux-armv7:
	@echo "==> Building for linux/arm"
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-armv7 ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && tar -czf $(APP_NAME)_$(VERSION)_linux-armv7.tar.gz $(APP_NAME)_$(VERSION)_linux-armv7
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-armv7 ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-armv7 ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-armv7 | cut -f1; else echo "N/A"; fi))"

# Windows builds
.PHONY: windows
windows: windows-amd64 windows-386

.PHONY: windows-amd64
windows-amd64:
	@echo "==> Building for windows/amd64"
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows-amd64.exe ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && zip -q $(APP_NAME)_$(VERSION)_windows-amd64.zip $(APP_NAME)_$(VERSION)_windows-amd64.exe
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows-amd64.exe ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows-amd64.exe ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows-amd64.exe | cut -f1; else echo "N/A"; fi))"

.PHONY: windows-386
windows-386:
	@echo "==> Building for windows/386"
	@CGO_ENABLED=0 GOOS=windows GOARCH=386 go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows-386.exe ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && zip -q $(APP_NAME)_$(VERSION)_windows-386.zip $(APP_NAME)_$(VERSION)_windows-386.exe
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows-386.exe ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows-386.exe ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows-386.exe | cut -f1; else echo "N/A"; fi))"

# macOS builds - fixed syntax
.PHONY: mac
mac: macos-amd64 macos-arm64

.PHONY: macos
macos: macos-amd64 macos-arm64

.PHONY: macos-amd64
macos-amd64:
	@echo "==> Building for darwin/amd64"
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_macos-amd64 ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && tar -czf $(APP_NAME)_$(VERSION)_macos-amd64.tar.gz $(APP_NAME)_$(VERSION)_macos-amd64
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_macos-amd64 ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_macos-amd64 ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_macos-amd64 | cut -f1; else echo "N/A"; fi))"

.PHONY: macos-arm64
macos-arm64:
	@echo "==> Building for darwin/arm64"
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_macos-arm64 ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && tar -czf $(APP_NAME)_$(VERSION)_macos-arm64.tar.gz $(APP_NAME)_$(VERSION)_macos-arm64
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_macos-arm64 ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_macos-arm64 ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_macos-arm64 | cut -f1; else echo "N/A"; fi))"

# OpenWRT/MIPS builds
.PHONY: openwrt
openwrt: openwrt-mips openwrt-mipsle

.PHONY: openwrt-mips
openwrt-mips:
	@echo "==> Building for linux/mips (GOMIPS=softfloat)"
	@CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-mips ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && tar -czf $(APP_NAME)_$(VERSION)_linux-mips.tar.gz $(APP_NAME)_$(VERSION)_linux-mips
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-mips ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-mips ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-mips | cut -f1; else echo "N/A"; fi))"

.PHONY: openwrt-mipsle
openwrt-mipsle:
	@echo "==> Building for linux/mipsle (GOMIPS=softfloat)"
	@CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-mipsle ./cmd/jellyfin-discovery-proxy
	@cd $(BUILD_DIR) && tar -czf $(APP_NAME)_$(VERSION)_linux-mipsle.tar.gz $(APP_NAME)_$(VERSION)_linux-mipsle
	@echo "   Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-mipsle ($(shell if [ -f $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-mipsle ]; then du -h $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux-mipsle | cut -f1; else echo "N/A"; fi))"

# Docker build (handled by GitHub Actions - see .github/workflows/docker.yml)
.PHONY: docker
docker:
	@echo "==> Docker builds are handled by GitHub Actions"
	@echo "   To build locally, use: docker build -t $(OWNER)/$(APP_NAME):$(VERSION) ."
	@echo "   To trigger CI build, push a tag: git tag v$(VERSION) && git push origin v$(VERSION)"
	@echo "   Or manually trigger: gh workflow run docker.yml"

# Generate checksums
.PHONY: checksums
checksums:
	@echo "==> Generating checksums"
	@cd $(BUILD_DIR) && sha256sum * > $(APP_NAME)_$(VERSION)_checksums.txt

# Show help message
.PHONY: help
help:
	@echo "Jellyfin Discovery Proxy Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all           Build for all platforms (default)"
	@echo "  clean         Clean the build directory"
	@echo "  linux         Build Linux binaries only"
	@echo "  windows       Build Windows binaries only"
	@echo "  mac, macos    Build macOS binaries only (both aliases work)"
	@echo "  openwrt       Build OpenWRT/MIPS binaries only"
	@echo "  docker        Build Docker image only"
	@echo "  help          Show this help message"
	@echo ""
	@echo "For more detailed information, see the README.md file."