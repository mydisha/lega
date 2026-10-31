VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build install test lint release release-notes release-gh

# Build ./lega with the version from git.
build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION:v%=%)" -o lega ./cmd/lega

# Build and put lega in ~/.local/bin, for trying out your own changes.
install: build
	install -D -m 0755 lega $(HOME)/.local/bin/lega

test:
	go vet ./...
	go test -race ./...

lint:
	golangci-lint run
	shellcheck install.sh scripts/*.sh

# Release from this machine:
#   make release BUMP=minor      (patch, minor or major)
#   make release TAG=v1.0.0-rc.1
release:
	scripts/release.sh $(or $(TAG),$(BUMP))

# Preview the notes of the next release.
release-notes:
	@scripts/release-notes.sh

# Release from GitHub instead: runs the Release workflow on main.
#   make release-gh BUMP=minor
release-gh:
	gh workflow run release.yml -R mydisha/lega --ref main -f bump=$(or $(BUMP),patch) $(if $(TAG),-f version=$(TAG))
