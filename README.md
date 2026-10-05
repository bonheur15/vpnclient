# VPN Gate Client

A fast Linux VPN client for the free [VPN Gate](https://www.vpngate.net/en/) network, with a
modern web UI. Single Go binary with the React UI embedded — no external services.

![stack](https://img.shields.io/badge/backend-Go-00ADD8) ![stack](https://img.shields.io/badge/ui-React%20%2B%20Vite-61DAFB)

## Features

- **Auto connect** — walks the best-ranked servers and connects to the first one that
  actually works (tunnel up **and** traffic verified through it).
- **Auto retry & reconnect** — if a server fails or the link drops, it immediately moves on
  or re-hunts for a new server; rounds back off and re-fetch the list when exhausted.
- **Country picking** — select one or more countries and auto-connect cycles through all
  servers in them until a stable one sticks.
- **Dead-server memory** — servers that repeatedly fail are marked *dead* and skipped for a
  cooldown period (configurable), so retries never waste time on them first. All
  success/failure history is persisted and folded into ranking.
- **Recommendations** — servers ranked by a blend of VPN Gate speed/score/ping and your own
  local connect history.
- **TCP pre-check** — 2.5s reachability probe before a full OpenVPN attempt, plus a bulk
  "Test reachability" sweep from the UI.
- **Live dashboard** — connection state, exit IP, uptime, live download/upload rates with
  sparklines (via the OpenVPN management interface), full activity log.
- **Server browser** — search, country filter, sorting, favorites, health badges,
  hide-dead toggle.
- Offline-friendly: the server list is cached on disk and loaded instantly on boot.

## Requirements

- Linux, Go ≥ 1.22, Node ≥ 20.19 (build only)
- `openvpn` installed: `sudo apt install openvpn` (Debian/Ubuntu) or `sudo dnf install openvpn`
- Root for the tunnel: run the binary with `sudo`, **or** allow passwordless openvpn:

  ```
  # /etc/sudoers.d/openvpn
  yourusername ALL=(root) NOPASSWD: /usr/sbin/openvpn
  ```

## Build & run

```bash
make            # builds web UI + Go binary → ./vpngate-client
sudo ./vpngate-client
# open http://127.0.0.1:8787
```

Flags:

| flag | default | |
|---|---|---|
| `-listen` | `127.0.0.1:8787` | web UI / API address |
| `-data` | `~/.config/vpngate-client` | settings, stats, list cache (kept under your user even via sudo) |

### Development

```bash
make dev
# backend: http://127.0.0.1:8787
# live-reloading UI: http://127.0.0.1:5173
```

`make dev` checks the required tools, installs the locked frontend dependencies when
needed, builds the Go backend, and runs it together with Vite. Press Ctrl+C to stop both.
Root is not required to browse servers; connecting a VPN still requires permission to
start OpenVPN as described above.

The development addresses can be overridden when a port is already in use:

```bash
VPNGATE_DEV_BACKEND_ADDR=127.0.0.1:8888 \
VPNGATE_DEV_FRONTEND_PORT=5174 make dev
```

The API proxy follows `VPNGATE_DEV_BACKEND_ADDR` automatically. Set
`VITE_API_PROXY_TARGET` only when proxying to a backend managed separately.

To run either side separately, use `make dev-backend` or `make dev-frontend`.

### Run as a service

```ini
# /etc/systemd/system/vpngate-client.service
[Unit]
Description=VPN Gate client
After=network-online.target

[Service]
ExecStart=/opt/vpngate-client/vpngate-client -listen 127.0.0.1:8787
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Enable auto-connect-on-startup in Settings and the service will bring the VPN up on boot.

## How server ranking works

Each server gets an effective score:

- VPN Gate metrics: line speed (45%), their quality score (20%), ping (15%)
- Your history: recent successes boost, consecutive failures penalize heavily,
  a failed reachability probe penalizes, favorites get a large boost
- `consecutive failures ≥ threshold` ⇒ **dead**: excluded from auto-connect during the
  cooldown window, then slowly re-probed (oldest failures first) if nothing else is left

A connection only counts as a success after the tunnel is up **and** an external IP check
succeeds through it — "connects but no internet" servers are marked failed automatically.

## Notes

- VPN Gate servers are run by volunteers; expect churn. The list auto-refreshes every 30 min.
- DNS: if `/etc/openvpn/update-resolv-conf` or `update-systemd-resolved` is installed, the
  client uses it to apply the VPN's DNS (toggle in Settings) to avoid DNS leaks.
- The web UI listens on localhost only by default; use `-listen 0.0.0.0:8787` deliberately.
