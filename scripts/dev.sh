#!/usr/bin/env bash

set -Eeuo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
web_dir="$project_dir/web"
backend_addr="${VPNGATE_DEV_BACKEND_ADDR:-127.0.0.1:8787}"
frontend_host="${VPNGATE_DEV_FRONTEND_HOST:-127.0.0.1}"
frontend_port="${VPNGATE_DEV_FRONTEND_PORT:-5173}"
data_dir="${VPNGATE_DEV_DATA_DIR:-$project_dir/.dev-data}"
api_proxy_target="${VITE_API_PROXY_TARGET:-http://$backend_addr}"
tmp_dir=""
backend_pid=""
frontend_pid=""

require_command() {
  local command_name="$1"
  local requirement="$2"

  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'error: %s is required.\n' "$requirement" >&2
    exit 1
  fi
}

stop_process() {
  local pid="$1"

  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}

cleanup() {
  local exit_status=$?
  trap - EXIT INT TERM
  stop_process "$frontend_pid"
  stop_process "$backend_pid"
  if [[ -n "$tmp_dir" && -d "$tmp_dir" ]]; then
    rm -rf -- "$tmp_dir"
  fi
  exit "$exit_status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

require_command go 'Go 1.22 or newer (https://go.dev/dl/)'
require_command node 'Node.js 20.19 or newer (https://nodejs.org/)'
require_command npm 'npm (included with Node.js)'

go_version="$(go env GOVERSION | sed 's/^go//')"
node_version="$(node --version | sed 's/^v//')"

if [[ "$(printf '%s\n' 1.22 "$go_version" | sort -V | head -n1)" != '1.22' ]]; then
  printf 'error: Go 1.22 or newer is required (found %s).\n' "$go_version" >&2
  exit 1
fi

if ! node -e 'const [m,n]=process.versions.node.split(".").map(Number); process.exit((m === 20 && n >= 19) || m >= 22 ? 0 : 1)'; then
  printf 'error: Node.js 20.19+ or 22.12+ is required (found %s).\n' "$node_version" >&2
  exit 1
fi

printf 'Using Go %s and Node.js %s\n' "$go_version" "$node_version"

if [[ ! -x "$web_dir/node_modules/.bin/vite" || \
      "$web_dir/package-lock.json" -nt "$web_dir/node_modules/.package-lock.json" ]]; then
  printf 'Installing frontend dependencies...\n'
  (cd "$web_dir" && npm ci)
fi

# main.go embeds web/dist even during development. Vite serves the live UI,
# but a minimal production build must exist for the Go compiler's embed step.
if [[ ! -f "$web_dir/dist/index.html" ]]; then
  printf 'Preparing embedded frontend assets...\n'
  (cd "$web_dir" && npm run build)
fi

mkdir -p "$data_dir"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/vpngate-client-dev.XXXXXX")"

printf 'Building development backend...\n'
(cd "$project_dir" && go build -o "$tmp_dir/vpngate-client" .)

printf 'Starting API at http://%s\n' "$backend_addr"
"$tmp_dir/vpngate-client" -listen "$backend_addr" -data "$data_dir" &
backend_pid=$!

printf 'Starting UI at http://%s:%s\n' "$frontend_host" "$frontend_port"
(
  cd "$web_dir"
  export VITE_API_PROXY_TARGET="$api_proxy_target"
  exec ./node_modules/.bin/vite \
    --host "$frontend_host" \
    --port "$frontend_port" \
    --strictPort
) &
frontend_pid=$!

set +e
wait -n "$backend_pid" "$frontend_pid"
exit_status=$?
set -e
exit "$exit_status"
