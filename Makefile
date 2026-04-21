APP_NAME  := vpsm
MODULE    := nathanbeddoewebdev/vpsm
BUILD_DIR := build

# Version info (override via env or CLI: make build VERSION=1.2.3)
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME = $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
REMOTE   ?= origin

# Optimised linker flags: strip debug info + embed version metadata
LDFLAGS := -s -w \
  -X $(MODULE)/cmd.Version=$(VERSION) \
  -X $(MODULE)/cmd.Commit=$(COMMIT) \
  -X $(MODULE)/cmd.BuildTime=$(BUILD_TIME)

# Default: build for the host platform
.PHONY: build
build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(APP_NAME) .
	@echo "Built $(BUILD_DIR)/$(APP_NAME)"

# Quick development build (no optimisation flags)
.PHONY: dev
dev:
	go build -o $(BUILD_DIR)/$(APP_NAME) .

# Run all tests
.PHONY: test
test:
	go test ./... -count=1

# Run tests with verbose output and race detector
.PHONY: test-verbose
test-verbose:
	go test ./... -v -count=1 -race

# Run go vet and staticcheck (if installed)
.PHONY: lint
lint:
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || true

# Remove build artefacts
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	go clean -cache -testcache

# Cross-compile for common targets
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: release
release:
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		output="$(BUILD_DIR)/$(APP_NAME)-$$os-$$arch$$ext"; \
		echo "Building $$output"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -trimpath -ldflags '$(LDFLAGS)' -o $$output . || exit 1; \
	done
	@echo "All release binaries in $(BUILD_DIR)/"

# Install to $GOPATH/bin (or $HOME/go/bin)
.PHONY: install
install:
	CGO_ENABLED=0 go install -trimpath -ldflags '$(LDFLAGS)' .

.PHONY: tag
tag:
	@[ -n "$(TAG)" ] || (echo "TAG is required (e.g. make tag TAG=v1.2.3)"; exit 1)
	git tag -a "$(TAG)" -m "Release $(TAG)"
	git push $(REMOTE) "$(TAG)"

# Auto-bump semver based on latest reachable tag, then tag and push
.PHONY: tag-patch tag-minor tag-major
tag-patch: BUMP := patch
tag-minor: BUMP := minor
tag-major: BUMP := major
tag-patch tag-minor tag-major:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Working tree is dirty; commit or stash before tagging."; exit 1; \
	fi
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	version=$${latest#v}; \
	version=$${version%%-*}; \
	major=$${version%%.*}; rest=$${version#*.}; \
	minor=$${rest%%.*}; patch=$${rest#*.}; \
	case "$(BUMP)" in \
		major) major=$$((major+1)); minor=0; patch=0 ;; \
		minor) minor=$$((minor+1)); patch=0 ;; \
		patch) patch=$$((patch+1)) ;; \
	esac; \
	new="v$$major.$$minor.$$patch"; \
	echo "Bumping $$latest -> $$new"; \
	git tag -a "$$new" -m "Release $$new"; \
	git push $(REMOTE) "$$new"

.PHONY: help
help:
	@echo "Usage:"
	@echo "  make build         Build optimised binary for host platform"
	@echo "  make dev           Quick development build (no stripping)"
	@echo "  make test          Run all tests"
	@echo "  make test-verbose  Run tests with -v and -race"
	@echo "  make lint          Run go vet (+ staticcheck if available)"
	@echo "  make clean         Remove build artefacts and caches"
	@echo "  make release       Cross-compile for linux/darwin/windows (amd64+arm64)"
	@echo "  make install       Install to GOPATH/bin"
	@echo "  make tag           Create and push a git tag (TAG=v1.2.3, REMOTE=origin)"
	@echo "  make tag-patch     Auto-bump patch from latest tag, then tag and push"
	@echo "  make tag-minor     Auto-bump minor from latest tag, then tag and push"
	@echo "  make tag-major     Auto-bump major from latest tag, then tag and push"
	@echo ""
	@echo "Override version:  make build VERSION=1.0.0"
