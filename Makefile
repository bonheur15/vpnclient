.PHONY: all deps web build go run dev dev-backend dev-frontend check-dev check-go check-node clean

GO ?= go
NPM ?= npm

all: build

web:
	cd web && $(NPM) ci && $(NPM) run build

# Install the exact frontend dependency versions from package-lock.json.
deps:
	cd web && $(NPM) ci

build: web
	$(GO) build -o vpngate-client .

# Rebuild only the Go side (web/dist must already exist)
go:
	$(GO) build -o vpngate-client .

run: build
	sudo ./vpngate-client

# Run the backend and Vite together. The helper installs missing frontend
# dependencies and reliably stops both processes when either one exits.
dev:
	./scripts/dev.sh

dev-backend: check-go
	$(GO) run . -data .dev-data

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
	rm -rf vpngate-client web/dist web/node_modules
