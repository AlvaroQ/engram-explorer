# Makefile for engram-explorer single-binary build
# Compatible with POSIX sh (git-bash on Windows, bash on Linux/macOS in CI).

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY  := engram-explorer
DIST_SRC := apps/frontend/dist
DIST_EMBED := internal/web/dist

.PHONY: all frontend embed build dev test clean seed-demo demo

## all: build the full single binary (default target)
all: build

## frontend: compile the React app (sourcemaps disabled for release)
frontend:
	RELEASE=1 pnpm -F @engram-explorer/frontend build

## embed: copy the compiled frontend into the Go embed directory.
##        Preserves the placeholder so `go build` always works even after clean.
embed:
	rm -rf $(DIST_EMBED)
	mkdir -p $(DIST_EMBED)
	cp -r $(DIST_SRC)/. $(DIST_EMBED)/

## build: frontend → embed → compile the Go binary
build: frontend embed
	CGO_ENABLED=0 go build \
		-ldflags "-X main.version=$(VERSION)" \
		-o $(BINARY) \
		./cmd/engram-explorer

## dev: run the Go binary in watch/dev mode (frontend dev server handled separately via pnpm dev)
dev:
	go run -ldflags "-X main.version=dev" ./cmd/engram-explorer

## test: run all Go tests
test:
	go test ./...

## seed-demo: generate the demo database in ./demo (use DEMO_OUT=<dir> to override)
seed-demo:
	go run ./cmd/seed-demo --out $(or $(DEMO_OUT),./demo) --force

## demo: seed the demo database and launch the binary against it
demo: seed-demo
	ENGRAM_DATA_DIR=$(or $(DEMO_OUT),./demo) go run ./cmd/engram-explorer

## clean: remove the compiled binary and the embedded dist copy.
##        The placeholder index.html is re-created so `go build` keeps working.
clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(DIST_EMBED)
	mkdir -p $(DIST_EMBED)
	printf '<!doctype html><title>engram-explorer</title>Frontend not built — run `make build`\n' \
		> $(DIST_EMBED)/index.html
