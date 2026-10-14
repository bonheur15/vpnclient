// Thin fetch wrappers around the Go backend.
async function req(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
  return data
}

export const api = {
  status: () => req('GET', '/api/status'),
  servers: () => req('GET', '/api/servers'),
  refreshServers: () => req('POST', '/api/servers/refresh'),
  recommendations: (countries = []) =>
    req('GET', `/api/recommendations${countries.length ? `?countries=${countries.join(',')}` : ''}`),
  connect: (id) => req('POST', '/api/connect', { id }),
  auto: (countries = []) => req('POST', '/api/auto', { countries }),
  disconnect: () => req('POST', '/api/disconnect'),
  ping: (ids = []) => req('POST', '/api/ping', { ids }),
  favorite: (id) => req('POST', '/api/favorite', { id }),
  settings: () => req('GET', '/api/settings'),
  saveSettings: (cfg) => req('PUT', '/api/settings', cfg),
  logs: () => req('GET', '/api/logs'),
  preflight: () => req('GET', '/api/preflight'),
  setup: () => req('GET', '/api/setup'),
  fixSetup: () => req('POST', '/api/setup/fix'),
  info: () => req('GET', '/api/info'),
  quit: () => req('POST', '/api/quit'),
}

// ---- formatters / helpers ----

export function fmtBytes(n) {
  if (n == null) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = Number(n)
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function fmtRate(bps) {
  if (bps == null) return '—'
  const mbit = (bps * 8) / 1e6
  if (mbit >= 1) return `${mbit.toFixed(1)} Mbit/s`
  return `${((bps * 8) / 1e3).toFixed(0)} kbit/s`
}

// Compact axis label: 0 / 500k / 1.5M (bits per second).
export function fmtRateAxis(bps) {
  const bits = bps * 8
  if (bits >= 1e6) return `${+(bits / 1e6).toFixed(1)}M`
  if (bits >= 1e3) return `${Math.round(bits / 1e3)}k`
  return `${Math.round(bits)}`
}

export function fmtDuration(ms) {
  if (ms <= 0) return '0s'
  const s = Math.floor(ms / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h > 0) return `${h}h ${m}m ${sec}s`
  if (m > 0) return `${m}m ${sec}s`
  return `${sec}s`
}

// Go's zero time ("0001-01-01…") means "unset".
export function parseTime(iso) {
  if (!iso) return null
  const t = new Date(iso).getTime()
  return Number.isNaN(t) || t < 946684800000 ? null : t
}

export function fmtCountdown(ms) {
  if (ms <= 0) return 'now'
  const m = Math.ceil(ms / 60000)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  return m % 60 ? `${h}h ${m % 60}m` : `${h}h`
}

export function fmtUptimeDays(ms) {
  const d = ms / 86400000
  if (d >= 1) return `${d.toFixed(1)}d`
  return `${(ms / 3600000).toFixed(1)}h`
}

export function timeAgo(iso) {
  if (!iso) return 'never'
  const s = (Date.now() - new Date(iso).getTime()) / 1000
  if (s < 0 || Number.isNaN(s)) return 'never'
  if (s < 60) return `${Math.floor(s)}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}
