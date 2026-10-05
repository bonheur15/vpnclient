import React, { useMemo, useState } from 'react'
import { Search, RefreshCw, Radar, Star, X } from 'lucide-react'
import { api, fmtUptimeDays, timeAgo } from '../api.js'
import { Flag, HealthBadge, ScoreBar } from './widgets.jsx'

const SORTS = {
  score: ['Best score', (a, b) => b.effScore - a.effScore],
  speed: ['Fastest', (a, b) => b.speedMbps - a.speedMbps],
  ping: ['Lowest ping', (a, b) => (a.pingMs || 9999) - (b.pingMs || 9999)],
  country: ['Country', (a, b) => a.country.localeCompare(b.country) || b.effScore - a.effScore],
  uptime: ['Longest uptime', (a, b) => b.uptimeMs - a.uptimeMs],
}

export default function Servers({ servers, status, onReload }) {
  const [q, setQ] = useState('')
  const [country, setCountry] = useState('')
  const [sort, setSort] = useState('score')
  const [hideDead, setHideDead] = useState(true)
  const [busy, setBusy] = useState('')

  const countries = useMemo(() => {
    const m = new Map()
    for (const s of servers) {
      const cur = m.get(s.countryCode) || { code: s.countryCode, name: s.country, n: 0 }
      cur.n++
      m.set(s.countryCode, cur)
    }
    return [...m.values()].sort((a, b) => b.n - a.n)
  }, [servers])

  const rows = useMemo(() => {
    let out = servers
    if (country) out = out.filter((s) => s.countryCode === country)
    if (hideDead) out = out.filter((s) => s.health !== 'dead')
    if (q) {
      const needle = q.toLowerCase()
      out = out.filter(
        (s) =>
          s.hostname.toLowerCase().includes(needle) ||
          s.ip.includes(needle) ||
          s.country.toLowerCase().includes(needle),
      )
    }
    return [...out].sort(SORTS[sort][1])
  }, [servers, q, country, sort, hideDead])

  const run = (label, fn) => async () => {
    setBusy(label)
    try {
      await fn()
    } catch (e) {
      alert(e.message)
    } finally {
      setBusy('')
    }
  }

  const connectedId = status.state !== 'disconnected' ? status.server?.id : null

  return (
    <div className="servers">
      <div className="toolbar card">
        <span className="input-wrap">
          <Search className="icon" />
          <input
            className="input"
            placeholder="Search host, IP or country…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </span>
        <select className="input" value={country} onChange={(e) => setCountry(e.target.value)}>
          <option value="">All countries ({servers.length})</option>
          {countries.map((c) => (
            <option key={c.code} value={c.code}>
              {c.name} ({c.n})
            </option>
          ))}
        </select>
        <select className="input" value={sort} onChange={(e) => setSort(e.target.value)}>
          {Object.entries(SORTS).map(([k, [label]]) => (
            <option key={k} value={k}>{label}</option>
          ))}
        </select>
        <label className="check">
          <input type="checkbox" checked={hideDead} onChange={(e) => setHideDead(e.target.checked)} />
          hide dead
        </label>
        <button
          className="btn"
          disabled={!!busy}
          onClick={run('refresh', async () => {
            await api.refreshServers()
            onReload()
          })}
        >
          <RefreshCw className="icon" /> {busy === 'refresh' ? 'Refreshing…' : 'Refresh list'}
        </button>
        <button
          className="btn"
          disabled={!!busy}
          title="TCP-probe the top servers to find reachable ones"
          onClick={run('ping', async () => {
            await api.ping(rows.slice(0, 60).filter((s) => s.proto === 'tcp').map((s) => s.id))
            onReload()
          })}
        >
          <Radar className="icon" /> {busy === 'ping' ? 'Probing…' : 'Test reachability'}
        </button>
      </div>

      <div className="card table-card">
        <div className="table-scroll">
        <table className="server-table">
          <thead>
            <tr>
              <th></th>
              <th>Country</th>
              <th>Host</th>
              <th>Endpoint</th>
              <th className="num">Speed</th>
              <th className="num">Ping</th>
              <th className="num">Users</th>
              <th className="num">Up</th>
              <th>Health</th>
              <th>Score</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => (
              <tr key={s.id} className={connectedId === s.id ? 'row-active' : ''}>
                <td>
                  <button
                    className={`star ${s.stats.favorite ? 'on' : ''}`}
                    title="favorite"
                    onClick={run('fav' + s.id, async () => {
                      await api.favorite(s.id)
                    })}
                  >
                    <Star className="icon" />
                  </button>
                </td>
                <td>
                  <span className="cell-country">
                    <Flag cc={s.countryCode} name={s.country} /> {s.countryCode}
                  </span>
                </td>
                <td className="host" title={`operator: ${s.operator || 'unknown'}`}>{s.hostname}</td>
                <td className="mono dim">
                  {s.ip}:{s.port}
                  <span className={`proto ${s.proto}`}>{s.proto}</span>
                  {s.stats.lastLatencyMs > 0 && <span className="lat ok"> {s.stats.lastLatencyMs}ms</span>}
                  {s.stats.lastLatencyMs < 0 && <span className="lat bad"> <X className="icon" /></span>}
                </td>
                <td className="num">{s.speedMbps} <span className="dim">Mbps</span></td>
                <td className="num">{s.pingMs ? `${s.pingMs}ms` : '—'}</td>
                <td className="num">{s.sessions}</td>
                <td className="num" title="server uptime">{fmtUptimeDays(s.uptimeMs)}</td>
                <td>
                  <HealthBadge health={s.health} />
                  {s.stats.failures > 0 && (
                    <div
                      className="dim tiny"
                      title={`${s.stats.successes} ok / ${s.stats.failures} failed, last fail ${timeAgo(s.stats.lastFailure)}`}
                    >
                      {s.stats.successes} ok / {s.stats.failures} fail
                    </div>
                  )}
                </td>
                <td><ScoreBar score={s.effScore} /></td>
                <td>
                  {connectedId === s.id ? (
                    <button className="btn small danger" onClick={run('dc', () => api.disconnect())}>
                      Disconnect
                    </button>
                  ) : (
                    <button
                      className="btn small"
                      disabled={!!busy}
                      onClick={run('c' + s.id, () => api.connect(s.id))}
                    >
                      Connect
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        </div>
        {rows.length === 0 && (
          <p className="dim center pad">
            No servers match. {servers.length === 0 && 'Try “Refresh list”.'}
          </p>
        )}
        <p className="dim small pad-h">{rows.length} of {servers.length} servers shown</p>
      </div>
    </div>
  )
}
