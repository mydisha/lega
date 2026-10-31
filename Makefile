VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint release

# Build ./lega with the version from git.
build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION:v%=%)" -o lega ./cmd/lega

test:
	go vet ./...
	go test -race ./...

lint:
	golangci-lint run
	shellcheck install.sh scripts/*.sh

# make release TAG=v0.2.0 tags main and pushes the tag.
release:
	scripts/release.sh $(TAG)
