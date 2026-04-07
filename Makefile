.PHONY: build build-cli build-server build-mcp test vet fmt clean release

BIN_DIR := bin
DIST_DIR := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Platforms for cross-compile release.
RELEASE_PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

build: build-cli build-server build-mcp

build-cli:
	go build -o $(BIN_DIR)/dits ./cmd/dits

build-server:
	go build -o $(BIN_DIR)/dits-server ./cmd/dits-server

build-mcp:
	go build -o $(BIN_DIR)/dits-mcp ./cmd/dits-mcp

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)

# release cross-compiles all three binaries for every platform in
# RELEASE_PLATFORMS and packages them into archives under dist/.
# tar.gz for unix targets, zip for windows. SHA256SUMS is emitted alongside.
release:
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@set -e; \
	for p in $(RELEASE_PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "==> building $$os/$$arch"; \
		out=$(DIST_DIR)/$$os-$$arch; \
		mkdir -p $$out; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -o $$out/dits$$ext ./cmd/dits; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -o $$out/dits-server$$ext ./cmd/dits-server; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -o $$out/dits-mcp$$ext ./cmd/dits-mcp; \
		archive=dits-$(VERSION)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then \
			(cd $(DIST_DIR) && zip -qr $$archive.zip $$os-$$arch); \
		else \
			tar -C $(DIST_DIR) -czf $(DIST_DIR)/$$archive.tar.gz $$os-$$arch; \
		fi; \
	done
	@cd $(DIST_DIR) && (shasum -a 256 *.tar.gz *.zip 2>/dev/null || sha256sum *.tar.gz *.zip 2>/dev/null) > SHA256SUMS
	@echo "==> release artifacts in $(DIST_DIR)/"
