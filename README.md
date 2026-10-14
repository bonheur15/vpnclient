# VPN Gate Client

A fast **Windows and Linux** VPN client for the free [VPN Gate](https://www.vpngate.net/en/)
network, with a modern UI. Single self-contained binary — it sets up everything it needs
(OpenVPN, the network driver, permissions) on first run.

![stack](https://img.shields.io/badge/backend-Go-00ADD8) ![stack](https://img.shields.io/badge/ui-React%20%2B%20Tailwind-61DAFB)

**Download:** [latest release](https://github.com/bonheur15/vpnclient/releases/latest) ·
**Website:** [`docs/index.html`](docs/index.html)

## Features

- **Zero setup** — on Windows the app downloads the official OpenVPN installer (pinned
  version, SHA-256 verified), installs it silently with its TAP driver, and creates the
  network adapter. On Linux it installs `openvpn` with your package manager and, with one
  polkit prompt, lets itself start OpenVPN without asking for your password again.
- **Auto connect** — walks the best-ranked servers and connects to the first one that
  actually works (tunnel up **and** traffic verified through it).
- **Auto-rotate** — optionally switch to a fresh server every 1–24 hours. The minimum is
  **1 hour** (enforced by the backend, not just the UI) so volunteer servers aren't hammered.
  Servers you rotated away from go to the back of the queue for 6 hours.
- **Auto retry & reconnect** — if a server fails or the link drops, it immediately moves on
  or re-hunts for a new server; rounds back off and re-fetch the list when exhausted.
- **Country picking** — select one or more countries and auto-connect cycles through all
  servers in them until a stable one sticks.
- **Dead-server memory** — servers that repeatedly fail are marked *dead* and skipped for a
  cooldown period (configurable). All success/failure history is persisted and folded into ranking.
- **Recommendations** — servers ranked by a blend of VPN Gate speed/score/ping and your own
  local connect history.
- **Live dashboard** — connection state, exit IP, uptime, live download/upload throughput
  chart (via the OpenVPN management interface), full activity log.
- **Server browser** — search, country filter, sorting, favorites, health badges, reachability probe.
- **Desktop app feel** — native window on Windows (WebView2, falls back to the browser),
  single instance (launching again focuses the running app), Quit from the sidebar.
- Offline-friendly: the server list is cached on disk and loaded instantly on boot.

## Install & run

### Windows (10 / 11)

Download `vpngate-client-windows-amd64.exe` (or `-arm64.exe`) and run it. Accept the
administrator prompt — creating a tunnel adapter requires it. First launch installs
OpenVPN automatically (progress is shown in the app). That's it.

- The exe isn't code-signed yet, so SmartScreen may warn: *More info → Run anyway*.
- Closing the window quits the app and disconnects the VPN.
- Settings, stats and the log live in `%APPDATA%\vpngate-client\` (`vpngate-client.log`).

### Linux

```bash
tar xzf vpngate-client-linux-amd64.tar.gz
./vpngate-client/vpngate-client          # opens http://127.0.0.1:8787 in your browser
```

If OpenVPN is missing or the app can't start it as root, the UI offers a one-click
**Install & set up / Grant access**. It runs a privileged helper via `pkexec` (one password
prompt) that:

1. installs `openvpn` with your package manager (apt, dnf, yum, pacman, zypper, apk or xbps);
2. installs a root-owned launcher at `/usr/local/libexec/vpngate-client/vpngate-openvpn-launch`;
3. writes `/etc/sudoers.d/vpngate-client`, letting *your user* run *only that launcher*
   without a password.

The launcher is a copy of the app that, because of its name, can do exactly one thing: read a
config on stdin, strip it to an allowlist of client directives (no scripts, plugins or file
paths), and run OpenVPN with the app's fixed arguments. So unlike a sudo rule for `openvpn`
itself, it doesn't hand out root. To revoke, delete both paths above. Running the app with
`sudo` also works (OpenVPN is then installed automatically, no prompt), as does an existing
manual `NOPASSWD` rule for openvpn.

Flags:

| flag | default | |
|---|---|---|
| `-listen` | `127.0.0.1:8787` | web UI / API address |
| `-data` | `~/.config/vpngate-client` (`%APPDATA%` on Windows) | settings, stats, list cache, log (kept under your user even via sudo) |
| `-no-ui` | `false` | don't open a window/browser on start (services, headless) |
| `-browser` | `false` | Windows: use the default browser instead of the app window |
| `-version` | | print the version |

## Build

Requires Go ≥ 1.22 and Node ≥ 20.19.

```bash
make            # builds web UI + Go binary → ./vpngate-client
make dist       # everything a release ships → dist/ (Linux + Windows, amd64 + arm64)
make windows    # just the Windows exes (embeds icon + admin manifest via go-winres)
```

Windows builds are pure Go (no cgo), so they cross-compile from Linux.

## Releasing

Push a version tag and GitHub Actions ([`.github/workflows/release.yml`](.github/workflows/release.yml))
builds everything and publishes a GitHub Release with the binaries and `SHA256SUMS.txt`:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Tags containing a hyphen (e.g. `v1.1.0-rc1`) are published as pre-releases. Asset names are
version-free, so `releases/latest/download/<asset>` links (used by the website) always
point at the newest build.

## Website

`docs/index.html` is a static landing page (Tailwind via CDN, no build step). To publish it,
enable GitHub Pages: *Settings → Pages → Deploy from a branch → `main` / `/docs`*.

## Development

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

## Run as a service (Linux)

```ini
# /etc/systemd/system/vpngate-client.service
[Unit]
Description=VPN Gate client
After=network-online.target

[Service]
ExecStart=/opt/vpngate-client/vpngate-client -listen 127.0.0.1:8787 -no-ui
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
- DNS (Linux): if `/etc/openvpn/update-resolv-conf` or `update-systemd-resolved` is installed, the
  client uses it to apply the VPN's DNS (toggle in Settings) to avoid DNS leaks. On Windows,
  OpenVPN applies the pushed DNS servers to the adapter itself.
- Windows OpenVPN version: pinned in `internal/setup/setup_windows.go`. To upgrade, bump
  `openvpnVersion` and both SHA-256 hashes (verify the `.asc` signatures first).
- Auto-rotate briefly drops the tunnel while switching servers (there's no kill switch), so
  your real IP can be exposed for a few seconds during each rotation.
- The local API rejects cross-origin state-changing requests, so other websites open in
  your browser can't drive the app.
- The web UI listens on localhost only by default; use `-listen 0.0.0.0:8787` deliberately.
