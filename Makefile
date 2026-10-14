.PHONY: all deps web build go run dev dev-backend dev-frontend check-dev check-go check-node clean windows winres dist

GO ?= go
NPM ?= npm
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GO_WINRES ?= go run github.com/tc-hib/go-winres@v0.3.3

all: build

web:
	cd web && $(NPM) ci && $(NPM) run build

# Install the exact frontend dependency versions from package-lock.json.
deps:
	cd web && $(NPM) ci

build: web
	$(GO) build -ldflags "$(LDFLAGS)" -o vpngate-client .

# Rebuild only the Go side (web/dist must already exist)
go:
	$(GO) build -ldflags "$(LDFLAGS)" -o vpngate-client .

# Windows resources: icon, version info and the manifest that makes Windows
# ask for admin rights on launch (needed to create the tunnel adapter).
winres:
	$(GO_WINRES) make --arch amd64,arm64 --product-version "$(VERSION:v%=%)" --file-version "$(VERSION:v%=%)" || \
		$(GO_WINRES) make --arch amd64,arm64

# Cross-compile the Windows app (no console window: it opens its own UI).
windows: winres
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS) -H windowsgui" -o dist/vpngate-client-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS) -H windowsgui" -o dist/vpngate-client-windows-arm64.exe .
	rm -f rsrc_windows_*.syso

# Everything a release ships, in dist/ (CI runs this on tag pushes).
dist: web windows
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o dist/vpngate-client-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o dist/vpngate-client-linux-arm64 .

run: build
	sudo ./vpngate-client

# Run the backend and Vite together. The helper installs missing frontend
# dependencies and reliably stops both processes when either one exits.
dev:
	./scripts/dev.sh

dev-backend: check-go
	$(GO) run . -data .dev-data -no-ui

dev-frontend: check-node
	@test -x web/node_modules/.bin/vite || $(MAKE) deps
	cd web && $(NPM) run dev

check-dev: check-go check-node

check-go:
	@command -v $(GO) >/dev/null || { echo "error: Go 1.22+ is required (https://go.dev/dl/)" >&2; exit 1; }
	@version=$$($(GO) env GOVERSION | sed 's/^go//'); first=$$(printf '%s\n' 1.22 "$$version" | sort -V | head -n1); test "$$first" = 1.22 || { echo "error: Go 1.22+ is required (found $$version)" >&2; exit 1; }

check-node:
	@command -v node >/dev/null || { echo "error: Node.js 20.19+ is required (https://nodejs.org/)" >&2; exit 1; }
	@node -e 'const [m,n]=process.versions.node.split(".").map(Number); process.exit((m === 20 && n >= 19) || m >= 22 ? 0 : 1)' || { echo "error: Node.js 20.19+ or 22.12+ is required" >&2; exit 1; }
	@command -v $(NPM) >/dev/null || { echo "error: npm is required (included with Node.js)" >&2; exit 1; }

clean:
	rm -rf vpngate-client dist web/dist web/node_modules rsrc_windows_*.syso
