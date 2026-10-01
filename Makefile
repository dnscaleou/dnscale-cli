.PHONY: build check cross-check release

VERSION ?= 1.0.0
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST_DIR ?= dist
export GOCACHE ?= $(abspath .gocache)
export GOWORK := off
export GOTOOLCHAIN ?= go$(shell cat .go-version)

build:
	go build -mod=readonly -trimpath -ldflags="$(LDFLAGS)" -o bin/dnscale ./cmd/dnscale

check:
	@test -z "$$(gofmt -l cmd internal)" || { gofmt -l cmd internal; exit 1; }
	go test -mod=readonly -race ./...
	go vet -mod=readonly ./...
	go mod verify

# Compile only; these artifacts are not published or signed releases.
cross-check:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o "$(DIST_DIR)/dnscale-linux-amd64" ./cmd/dnscale
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=readonly -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o "$(DIST_DIR)/dnscale-linux-arm64" ./cmd/dnscale
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -mod=readonly -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o "$(DIST_DIR)/dnscale-darwin-amd64" ./cmd/dnscale
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=readonly -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o "$(DIST_DIR)/dnscale-darwin-arm64" ./cmd/dnscale
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=readonly -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o "$(DIST_DIR)/dnscale-windows-amd64.exe" ./cmd/dnscale

release:
	python3 scripts/package-release.py "$(VERSION)"
