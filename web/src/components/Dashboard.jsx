import React, { useEffect, useState } from 'react'
import {
  Power, ArrowDown, ArrowUp, Copy, Check, RefreshCw, ChevronRight, Sparkles, ShieldAlert, ScrollText,
} from 'lucide-react'
import { api, fmtBytes, fmtRate, fmtDuration, fmtCountdown, parseTime, timeAgo } from '../api.js'
import { Flag, HealthBadge, StatusPill, Spinner, Empty, useAction, useNow } from './widgets.jsx'
import CountryPicker from './CountryPicker.jsx'
import ThroughputChart from './ThroughputChart.jsx'

export default function Dashboard({ status, settings, servers, logs, rateHistory, ready, onSaveSettings, onNavigate }) {
  const [recs, setRecs] = useState([])
  const [run, busy] = useAction()
  const countries = settings?.countries || []
  const isIdle = status.state === 'disconnected'
  const isBusy = status.state === 'connecting' || status.state === 'retrying'
  const connected = status.state === 'connected'
  const now = useNow(connected || !!status.nextRotateAt)

  useEffect(() => {
    const load = () => api.recommendations(countries).then(setRecs).catch(() => {})
    load()
    const t = setInterval(load, 60000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(countries), status.state, servers.length])

  const toggle = run('main', () => (isIdle ? api.auto(countries) : api.disconnect()))
  const setCountries = (cs) => onSaveSettings({ ...settings, countries: cs })

  return (
    <div className="space-y-5">
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
        {/* ---- hero ---- */}
        <section className="card relative overflow-hidden p-6">
          <HeroGlow state={status.state} />
          <div className="relative flex items-center justify-between">
            <StatusPill status={status} />
            {status.mode && !isIdle && <span className="label-xs">{status.mode} mode</span>}
          </div>
          <div className="relative flex flex-col items-center pt-6 pb-2">
            <ConnectButton state={status.state} disabled={!!busy || (!ready && isIdle)} onClick={toggle} />
            <h2 className="mt-6 text-xl font-semibold tracking-tight text-zinc-50">
              {connected && 'Your connection is secure'}
              {status.state === 'connecting' && `Connecting${status.total ? ` · attempt ${status.attempt} of ${status.total}` : '…'}`}
              {status.state === 'retrying' && 'Looking for a working server…'}
              {isIdle && 'You are not protected'}
            </h2>
            <p className="mt-1 max-w-md text-center text-[13px] text-zinc-400 first-letter:uppercase">
              {!ready && isIdle ? 'Finish setup to connect.' : status.message}
            </p>
          </div>
          <div className="relative mx-auto mt-5 max-w-sm">
            <div className="label-xs mb-1.5 px-1">Location</div>
            <CountryPicker servers={servers} value={countries} onChange={setCountries} />
          </div>
        </section>

        {/* ---- details ---- */}
        <section className="card flex flex-col p-6">
          <h3 className="mb-4 text-sm font-semibold text-zinc-100">Connection</h3>
          {status.server && !isIdle ? (
            <ServerHeader server={status.server} />
          ) : (
            <div className="flex items-center gap-3 rounded-xl bg-rose-400/[0.06] p-3.5 ring-1 ring-rose-400/15">
              <ShieldAlert className="icon text-2xl text-rose-300" />
              <div>
                <div className="font-medium text-rose-100">Your real IP address is visible</div>
                <div className="text-xs text-rose-200/60">Connect to route your traffic through a volunteer VPN Gate server.</div>
              </div>
            </div>
          )}
          <dl className="mt-4 divide-y divide-white/[0.05] text-[13px]">
            <Detail label="Exit IP">
              {status.publicIp ? <CopyText text={status.publicIp} /> : <span className="text-zinc-500">—</span>}
            </Detail>
            <Detail label="Endpoint">
              {status.server && !isIdle ? (
                <span className="font-mono text-xs text-zinc-300">
                  {status.server.ip}:{status.server.port}
                  <span className="ml-1.5 rounded bg-white/[0.06] px-1 py-px text-[10px] font-semibold uppercase text-zinc-400">{status.server.proto}</span>
                </span>
              ) : (
                <span className="text-zinc-500">—</span>
              )}
            </Detail>
            <Detail label="Connected for">
              {connected && parseTime(status.connectedAt) ? (
                <span className="tabular-nums text-zinc-200">{fmtDuration(now - parseTime(status.connectedAt))}</span>
              ) : (
                <span className="text-zinc-500">—</span>
              )}
            </Detail>
            <Detail label="Auto-rotate">
              {settings?.autoRotate ? (
                <span className="flex items-center gap-1.5 text-zinc-200">
                  <RefreshCw className="icon text-brand-400" />
                  {connected && parseTime(status.nextRotateAt)
                    ? <span>next in <span className="tabular-nums">{fmtCountdown(parseTime(status.nextRotateAt) - now)}</span></span>
                    : `every ${fmtCountdown(settings.rotateIntervalMin * 60000)}`}
                </span>
              ) : (
                <button className="flex items-center gap-1 text-zinc-400 hover:text-brand-400" onClick={() => onNavigate('settings')}>
                  Off <ChevronRight className="icon" />
                </button>
              )}
            </Detail>
          </dl>
          {status.lastError && isIdle && (
            <p className="mt-auto pt-4 text-xs text-zinc-500">Last error: {status.lastError}</p>
          )}
        </section>
      </div>

      {connected && (
        <>
          <section className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <Tile icon={ArrowDown} label="Download" value={fmtRate(status.rxRate)} sub={`${fmtBytes(status.rxBytes)} received`} dot="var(--color-series-down)" />
            <Tile icon={ArrowUp} label="Upload" value={fmtRate(status.txRate)} sub={`${fmtBytes(status.txBytes)} sent`} dot="var(--color-series-up)" />
            <Tile label="Data this session" value={fmtBytes((status.rxBytes || 0) + (status.txBytes || 0))} sub="since connecting" />
            <Tile label="Server speed" value={`${status.server?.speedMbps ?? '—'} Mbps`} sub={`${status.server?.sessions ?? 0} users on server`} />
          </section>
          <section className="card animate-enter p-5">
            <ThroughputChart history={rateHistory} />
          </section>
        </>
      )}

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        <section className="card p-5">
          <div className="mb-3 flex items-center justify-between">
            <div>
              <h3 className="text-sm font-semibold text-zinc-100">Recommended servers</h3>
              <p className="text-xs text-zinc-500">Ranked by speed, reliability and your connection history</p>
            </div>
            <button className="btn btn-ghost btn-sm" onClick={() => onNavigate('servers')}>
              All servers <ChevronRight className="icon" />
            </button>
          </div>
          {recs.length === 0 ? (
            <Empty icon={Sparkles} title="No recommendations yet">Refresh the server list from the Servers page.</Empty>
          ) : (
            <ul className="-mx-2">
              {recs.map((r) => {
                const active = status.server?.id === r.id && !isIdle
                return (
                  <li key={r.id} className={`group flex items-center gap-3 rounded-xl px-2 py-2.5 transition hover:bg-white/[0.03] ${active ? 'bg-emerald-400/[0.05]' : ''}`}>
                    <Flag cc={r.countryCode} name={r.country} className="text-xl" />
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-medium text-zinc-100">{r.hostname}</div>
                      <div className="truncate text-xs text-zinc-500">
                        {r.country} · {r.speedMbps} Mbps · {r.pingMs ? `${r.pingMs} ms` : 'ping —'}
                        {r.stats.successes > 0 && ` · worked ${timeAgo(r.stats.lastSuccess)}`}
                      </div>
                    </div>
                    <HealthBadge health={r.health} />
                    {active ? (
                      <span className="w-[72px] text-center text-xs font-medium text-emerald-300">Active</span>
                    ) : (
                      <button className="btn btn-sm w-[72px]" disabled={!!busy || isBusy || !ready} onClick={run(r.id, () => api.connect(r.id))}>
                        {busy === r.id ? <Spinner /> : 'Connect'}
                      </button>
                    )}
                  </li>
                )
              })}
            </ul>
          )}
        </section>

        <section className="card flex flex-col p-5">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-zinc-100">Recent activity</h3>
            <button className="btn btn-ghost btn-sm" onClick={() => onNavigate('activity')}>
              View log <ChevronRight className="icon" />
            </button>
          </div>
          <ActivityPreview logs={logs} />
        </section>
      </div>
    </div>
  )
}

function HeroGlow({ state }) {
  const color = {
    connected: 'from-emerald-400/20',
    connecting: 'from-amber-400/15',
    retrying: 'from-amber-400/15',
  }[state] || 'from-brand-500/10'
  return (
    <div
      aria-hidden
      className={`pointer-events-none absolute -top-40 left-1/2 h-80 w-[36rem] -translate-x-1/2 rounded-full bg-radial ${color} to-transparent to-70% transition-colors duration-700`}
    />
  )
}

function ConnectButton({ state, disabled, onClick }) {
  const connected = state === 'connected'
  const busy = state === 'connecting' || state === 'retrying'
  return (
    <div className="relative grid size-44 place-items-center">
      {connected && (
        <>
          <span className="absolute inset-3 animate-ripple rounded-full border border-emerald-400/40" />
          <span className="absolute inset-3 animate-ripple rounded-full border border-emerald-400/30 [animation-delay:1.3s]" />
        </>
      )}
      {busy && (
        <svg className="absolute inset-0 animate-spin-slow" viewBox="0 0 176 176" aria-hidden>
          <circle cx="88" cy="88" r="82" fill="none" stroke="rgb(251 191 36 / .12)" strokeWidth="3" />
          <circle cx="88" cy="88" r="82" fill="none" stroke="#fbbf24" strokeWidth="3" strokeLinecap="round" strokeDasharray="120 400" />
        </svg>
      )}
      <span
        className={`absolute inset-4 rounded-full transition-all duration-500 ${
          connected ? 'bg-emerald-400/10 ring-1 ring-emerald-400/30' : busy ? 'bg-amber-400/[0.06] ring-1 ring-amber-400/20' : 'bg-white/[0.02] ring-1 ring-white/[0.06]'
        }`}
      />
      <button
        onClick={onClick}
        disabled={disabled}
        aria-label={connected ? 'Disconnect' : busy ? 'Cancel' : 'Connect'}
        className={`group relative grid size-28 place-items-center rounded-full text-5xl transition-all duration-300 active:scale-95 disabled:opacity-50 ${
          connected
            ? 'bg-gradient-to-br from-emerald-400 to-teal-500 text-white shadow-[0_0_60px_-8px_rgb(52_211_153/0.7)] hover:shadow-[0_0_70px_-6px_rgb(52_211_153/0.85)]'
            : busy
              ? 'bg-ink-700 text-amber-300 ring-1 ring-amber-400/30 hover:bg-ink-600'
              : 'bg-gradient-to-br from-ink-600 to-ink-750 text-zinc-300 ring-1 ring-white/10 hover:text-brand-400 hover:ring-brand-500/50 hover:shadow-[0_0_50px_-10px_rgb(79_140_255/0.7)]'
        }`}
      >
        <Power className="icon transition-transform duration-300 group-hover:scale-105" strokeWidth={2.2} />
      </button>
      <span className="absolute -bottom-1 text-[11px] font-medium text-zinc-500">
        {connected ? 'Tap to disconnect' : busy ? 'Tap to cancel' : 'Tap to connect'}
      </span>
    </div>
  )
}

function ServerHeader({ server }) {
  return (
    <div className="flex items-center gap-3.5 rounded-xl bg-white/[0.03] p-3.5 ring-1 ring-white/[0.05]">
      <Flag cc={server.countryCode} name={server.country} className="text-3xl" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-[15px] font-semibold text-zinc-50">{server.country}</div>
        <div className="truncate text-xs text-zinc-400">{server.hostname} · {server.operator || 'volunteer server'}</div>
      </div>
      <HealthBadge health={server.health} />
    </div>
  )
}

function Detail({ label, children }) {
  return (
    <div className="flex items-center justify-between gap-4 py-2.5">
      <dt className="text-zinc-500">{label}</dt>
      <dd className="min-w-0 truncate">{children}</dd>
    </div>
  )
}

function CopyText({ text }) {
  const [done, setDone] = useState(false)
  return (
    <button
      className="group flex items-center gap-1.5 font-mono text-xs text-zinc-100"
      onClick={() => {
        navigator.clipboard?.writeText(text).then(() => {
          setDone(true)
          setTimeout(() => setDone(false), 1500)
        })
      }}
      title="Copy"
    >
      {text}
      {done ? <Check className="icon text-emerald-400" /> : <Copy className="icon text-zinc-600 group-hover:text-zinc-300" />}
    </button>
  )
}

function Tile({ icon: Icon, label, value, sub, dot }) {
  return (
    <div className="card animate-enter p-4">
      <div className="flex items-center gap-1.5 text-xs text-zinc-400">
        {dot && <span className="size-2 rounded-full" style={{ background: dot }} />}
        {Icon && <Icon className="icon text-zinc-500" />}
        {label}
      </div>
      <div className="mt-1.5 text-[22px] font-semibold tracking-tight text-zinc-50">{value}</div>
      <div className="mt-0.5 text-xs text-zinc-500">{sub}</div>
    </div>
  )
}

function ActivityPreview({ logs }) {
  const items = logs.filter((l) => l.line.startsWith('[app]')).slice(-7).reverse()
  if (items.length === 0) return <Empty icon={ScrollText} title="Nothing yet" />
  return (
    <ol className="relative space-y-3 border-l border-white/[0.07] pl-4">
      {items.map((l, i) => {
        const text = l.line.replace(/^\[app\]\s*/, '')
        const tone = /fail|error|lost|unreachable|WARNING/i.test(text)
          ? 'bg-rose-400'
          : /connected to|refreshed|enabled/i.test(text)
            ? 'bg-emerald-400'
            : 'bg-zinc-600'
        return (
          <li key={`${l.time}-${i}`} className="relative">
            <span className={`absolute top-1.5 -left-[19.5px] size-1.5 rounded-full ring-4 ring-ink-800 ${tone}`} />
            <p className="line-clamp-2 text-[12.5px] text-zinc-300">{text}</p>
            <p className="text-[11px] text-zinc-600">{new Date(l.time).toLocaleTimeString()}</p>
          </li>
        )
      })}
    </ol>
  )
}

