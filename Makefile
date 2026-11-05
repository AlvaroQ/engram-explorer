# Makefile for engram-explorer single-binary build
# Compatible with POSIX sh (git-bash on Windows, bash on Linux/macOS in CI).

# Same format as GoReleaser's {{.Version}}: the tag without its leading "v".
VERSION ?= $(or $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//'),dev)
BINARY  := engram-explorer
DIST_SRC := apps/frontend/dist
DIST_EMBED := internal/web/dist

.PHONY: all frontend embed templ build run dev test clean seed-demo demo

## all: build the full single binary (default target)
all: build

# Pinned templ generator (must match the github.com/a-h/templ version in go.mod
# and TEMPL_VERSION in .github/workflows/ci.yml).
TEMPL_VERSION := v0.3.1001
TEMPL := go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION)

## templ: generate Go code from .templ templates (run from the repo root so the
##        generated file paths match the CI drift check)
templ:
	$(TEMPL) generate

## frontend: compile the React island bundles, then the Tailwind CSS the
##           templ UI consumes (scans the .templ files; output embedded via the
##           ui package's //go:embed static).
frontend:
	RELEASE=1 pnpm -F @engram-explorer/frontend build
	pnpm -F @engram-explorer/frontend build:ui-css

## embed: copy the compiled frontend into the Go embed directory.
##        Preserves the placeholder so `go build` always works even after clean.
embed:
	rm -rf $(DIST_EMBED)
	mkdir -p $(DIST_EMBED)
	cp -r $(DIST_SRC)/. $(DIST_EMBED)/

## build: templ → frontend → embed → compile the Go binary
build: templ frontend embed
	CGO_ENABLED=0 go build \
		-ldflags "-X main.version=$(VERSION)" \
		-o $(BINARY) \
		./cmd/engram-explorer

## run: full build (frontend → embed → binary) then launch it against ~/.engram/engram.db
run: build
	./$(BINARY)

## dev: run the Go binary in development mode (detailed errors, Vite dev origins trusted;
##      frontend dev server handled separately via pnpm dev)
dev:
	ENGRAM_DASH_ENV=development go run -ldflags "-X main.version=dev" ./cmd/engram-explorer

## test: run all Go tests
test:
	go test ./...

## seed-demo: generate the demo dataset in ./demo — engram.db plus sample Claude
##            Code transcripts in ./demo/claude and Codex CLI rollouts in
##            ./demo/codex (use DEMO_OUT=<dir> to override)
seed-demo:
	go run ./cmd/seed-demo --out $(or $(DEMO_OUT),./demo) --force

## demo: seed the demo dataset and run the normal binary read-only against it.
##       Engram writes answer 503; settings persist in ./demo/config.json and
##       nothing is read from ~/.engram, ~/.claude/projects or ~/.codex.
demo: seed-demo
	ENGRAM_DATA_DIR=$(or $(DEMO_OUT),./demo) \
	CLAUDE_PROJECTS_DIR=$(or $(DEMO_OUT),./demo)/claude \
	CODEX_HOME=$(or $(DEMO_OUT),./demo)/codex \
	ENGRAM_DASH_READONLY=true \
	go run ./cmd/engram-explorer

## clean: remove the compiled binary and the embedded dist copy.
##        The placeholder index.html is re-created so `go build` keeps working.
clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(DIST_EMBED)
	mkdir -p $(DIST_EMBED)
	printf '<!doctype html><title>engram-explorer</title>Frontend not built — run `make build`\n' \
		> $(DIST_EMBED)/index.html
