import React, { useMemo, useState } from 'react'
import { Search, RefreshCw, Radar, Star, X, ServerOff, ArrowUpDown } from 'lucide-react'
import { api, fmtUptimeDays, timeAgo } from '../api.js'
import { Flag, HealthBadge, ScoreBar, Switch, Spinner, Empty, useAction, useToast } from './widgets.jsx'
import { useCountries } from './CountryPicker.jsx'

const SORTS = {
  score: ['Best match', (a, b) => b.effScore - a.effScore],
  speed: ['Fastest', (a, b) => b.speedMbps - a.speedMbps],
  ping: ['Lowest ping', (a, b) => (a.pingMs || 9999) - (b.pingMs || 9999)],
  country: ['Country', (a, b) => a.country.localeCompare(b.country) || b.effScore - a.effScore],
  uptime: ['Longest uptime', (a, b) => b.uptimeMs - a.uptimeMs],
}

// Columns that only fit on wide windows (users, uptime).
const WIDE_ONLY = [6, 7]

export default function Servers({ servers, status, ready, onReload }) {
  const [q, setQ] = useState('')
  const [country, setCountry] = useState('')
  const [sort, setSort] = useState('score')
  const [hideDead, setHideDead] = useState(true)
  const [favOnly, setFavOnly] = useState(false)
  const [run, busy] = useAction()
  const toast = useToast()
  const countries = useCountries(servers)

  const rows = useMemo(() => {
    let out = servers
    if (country) out = out.filter((s) => s.countryCode === country)
    if (hideDead) out = out.filter((s) => s.health !== 'dead')
    if (favOnly) out = out.filter((s) => s.stats.favorite)
    if (q) {
      const needle = q.toLowerCase()
      out = out.filter(
        (s) => s.hostname.toLowerCase().includes(needle) || s.ip.includes(needle) || s.country.toLowerCase().includes(needle),
      )
    }
    return [...out].sort(SORTS[sort][1])
  }, [servers, q, country, sort, hideDead, favOnly])

  const activeId = status.state !== 'disconnected' ? status.server?.id : null
  const tunnelBusy = status.state === 'connecting' || status.state === 'retrying'

  return (
    <div className="flex h-full flex-col gap-4">
      <div className="card flex flex-wrap items-center gap-2.5 p-3">
        <label className="flex min-w-56 flex-1 items-center gap-2 rounded-xl border border-white/[0.08] bg-ink-900 px-3 transition focus-within:border-brand-500/70 focus-within:ring-4 focus-within:ring-brand-500/15">
          <Search className="icon text-zinc-500" />
          <input
            className="w-full bg-transparent py-2 text-[13px] outline-none placeholder:text-zinc-500"
            placeholder="Search host, IP or country…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          {q && (
            <button onClick={() => setQ('')} className="text-zinc-500 hover:text-zinc-200" aria-label="Clear search">
              <X className="icon" />
            </button>
          )}
        </label>
        <select className="input cursor-pointer" value={country} onChange={(e) => setCountry(e.target.value)}>
          <option value="">All countries ({servers.length})</option>
          {countries.map((c) => (
            <option key={c.code} value={c.code}>
              {c.name} ({c.n})
            </option>
          ))}
        </select>
        <label className="relative flex items-center">
          <ArrowUpDown className="icon pointer-events-none absolute left-3 text-zinc-500" />
          <select className="input cursor-pointer pl-8" value={sort} onChange={(e) => setSort(e.target.value)}>
            {Object.entries(SORTS).map(([k, [label]]) => (
              <option key={k} value={k}>{label}</option>
            ))}
          </select>
        </label>
        <div className="flex items-center gap-4 px-1.5 text-xs text-zinc-400">
          <span className="flex items-center gap-2"><Switch checked={hideDead} onChange={setHideDead} label="Hide dead" /> Hide dead</span>
          <span className="flex items-center gap-2"><Switch checked={favOnly} onChange={setFavOnly} label="Favorites only" /> Favorites</span>
        </div>
        <div className="ml-auto flex gap-2">
          <button
            className="btn"
            disabled={!!busy}
            title="TCP-probe the top servers to find reachable ones"
            onClick={run('ping', async () => {
              const ids = rows.slice(0, 60).filter((s) => s.proto === 'tcp').map((s) => s.id)
              const r = await api.ping(ids)
              onReload()
              toast(`Probed ${r.probed} servers`, 'ok')
            })}
          >
            {busy === 'ping' ? <Spinner /> : <Radar className="icon" />} Test reachability
          </button>
          <button
            className="btn"
            disabled={!!busy}
            onClick={run('refresh', async () => {
              await api.refreshServers()
              onReload()
              toast('Server list refreshed', 'ok')
            })}
          >
            <RefreshCw className={`icon ${busy === 'refresh' ? 'animate-spin' : ''}`} /> Refresh
          </button>
        </div>
      </div>

      <div className="card flex min-h-0 flex-1 flex-col overflow-hidden">
        <div className="min-h-0 flex-1 overflow-auto">
          <table className="w-full border-separate border-spacing-0 text-[13px]">
            <thead className="sticky top-0 z-10 bg-ink-800/95 backdrop-blur">
              <tr className="text-left">
                {['', 'Location', 'Host', 'Endpoint', 'Speed', 'Ping', 'Users', 'Uptime', 'Health', 'Score', ''].map((h, i) => (
                  <th
                    key={i}
                    className={`label-xs border-b border-white/[0.06] px-3 py-3 font-semibold ${[4, 5, 6, 7].includes(i) ? 'text-right' : ''} ${WIDE_ONLY.includes(i) ? 'hidden 2xl:table-cell' : ''}`}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => {
                const active = activeId === s.id
                return (
                  <tr key={s.id} className={`group transition-colors ${active ? 'bg-emerald-400/[0.06]' : 'hover:bg-white/[0.025]'}`}>
                    <Td>
                      <button
                        className={`grid place-items-center rounded-md p-1 text-[15px] transition ${s.stats.favorite ? 'text-amber-300' : 'text-zinc-600 hover:text-zinc-300'}`}
                        title={s.stats.favorite ? 'Remove favorite' : 'Add to favorites'}
                        onClick={run('fav' + s.id, () => api.favorite(s.id))}
                      >
                        <Star className="icon" fill={s.stats.favorite ? 'currentColor' : 'none'} />
                      </button>
                    </Td>
                    <Td>
                      <span className="flex items-center gap-2 whitespace-nowrap">
                        <Flag cc={s.countryCode} name={s.country} className="text-base" />
                        <span className="max-w-32 truncate text-zinc-200">{s.country}</span>
                      </span>
                    </Td>
                    <Td className="max-w-40 truncate font-medium text-zinc-100" title={`operator: ${s.operator || 'unknown'}`}>{s.hostname}</Td>
                    <Td className="whitespace-nowrap font-mono text-xs text-zinc-400">
                      {s.ip}:{s.port}
                      <span className={`ml-1.5 rounded px-1 py-px text-[10px] font-semibold uppercase ${s.proto === 'tcp' ? 'bg-brand-500/15 text-brand-400' : 'bg-white/[0.06] text-zinc-400'}`}>
                        {s.proto}
                      </span>
                      {s.stats.lastLatencyMs > 0 && <span className="ml-1.5 text-emerald-300">{s.stats.lastLatencyMs}ms</span>}
                      {s.stats.lastLatencyMs < 0 && <span className="ml-1.5 text-rose-300">unreachable</span>}
                    </Td>
                    <Td className="text-right tabular-nums text-zinc-200">{s.speedMbps}<span className="ml-0.5 text-xs text-zinc-500">Mbps</span></Td>
                    <Td className="text-right tabular-nums text-zinc-300">{s.pingMs ? `${s.pingMs}ms` : '—'}</Td>
                    <Td className="hidden text-right tabular-nums text-zinc-300 2xl:table-cell">{s.sessions}</Td>
                    <Td className="hidden text-right tabular-nums text-zinc-400 2xl:table-cell">{fmtUptimeDays(s.uptimeMs)}</Td>
                    <Td>
                      <div className="flex flex-col items-start gap-0.5">
                        <HealthBadge health={s.health} />
                        {s.stats.failures > 0 && (
                          <span className="text-[10.5px] text-zinc-500" title={`last failure ${timeAgo(s.stats.lastFailure)}`}>
                            {s.stats.successes} ok · {s.stats.failures} failed
                          </span>
                        )}
                      </div>
                    </Td>
                    <Td><ScoreBar score={s.effScore} /></Td>
                    <Td className="pr-4 text-right">
                      {active ? (
                        <button className="btn btn-danger btn-sm w-[88px]" onClick={run('dc', () => api.disconnect())}>Disconnect</button>
                      ) : (
                        <button className="btn btn-sm w-[88px]" disabled={!!busy || tunnelBusy || !ready} onClick={run('c' + s.id, () => api.connect(s.id))}>
                          {busy === 'c' + s.id ? <Spinner /> : 'Connect'}
                        </button>
                      )}
                    </Td>
                  </tr>
                )
              })}
            </tbody>
          </table>
          {rows.length === 0 && (
            <Empty icon={ServerOff} title="No servers match">
              {servers.length === 0 ? 'The list is empty — try Refresh.' : 'Try clearing the search or filters.'}
            </Empty>
          )}
        </div>
        <div className="border-t border-white/[0.06] px-4 py-2.5 text-xs text-zinc-500">
          Showing {rows.length} of {servers.length} servers
        </div>
      </div>
    </div>
  )
}

function Td({ className = '', children, ...rest }) {
  return (
    <td className={`border-b border-white/[0.04] px-3 py-2.5 align-middle ${className}`} {...rest}>
      {children}
    </td>
  )
}
