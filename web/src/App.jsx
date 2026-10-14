import React, { useEffect, useRef, useState, useCallback } from 'react'
import { LayoutDashboard, Server as ServerIcon, Settings as SettingsIcon, ScrollText, TriangleAlert, Power, WifiOff, ArrowRight } from 'lucide-react'
import { api } from './api.js'
import Dashboard from './components/Dashboard.jsx'
import Servers from './components/Servers.jsx'
import Settings from './components/Settings.jsx'
import Activity from './components/Activity.jsx'
import Setup, { setupBlocking } from './components/Setup.jsx'
import { Flag, Logo, StatusPill, ToastProvider, Spinner, stateStyle } from './components/widgets.jsx'

const PAGES = [
  { id: 'dashboard', name: 'Dashboard', Icon: LayoutDashboard },
  { id: 'servers', name: 'Servers', Icon: ServerIcon },
  { id: 'activity', name: 'Activity', Icon: ScrollText },
  { id: 'settings', name: 'Settings', Icon: SettingsIcon },
]

export default function App() {
  return (
    <ToastProvider>
      <Shell />
    </ToastProvider>
  )
}

function Shell() {
  const [page, setPage] = useState('dashboard')
  const [status, setStatus] = useState({ state: 'disconnected', message: 'starting…' })
  const [servers, setServers] = useState([])
  const [settings, setSettings] = useState(null)
  const [logs, setLogs] = useState([])
  const [warnings, setWarnings] = useState([])
  const [setup, setSetup] = useState(null)
  const [setupHidden, setSetupHidden] = useState(false)
  const [info, setInfo] = useState(null)
  const [backendUp, setBackendUp] = useState(true)
  const [quitting, setQuitting] = useState(false)
  const [rateHistory, setRateHistory] = useState([])
  const esRef = useRef(null)

  const loadServers = useCallback(() => {
    api.servers().then(setServers).catch(() => {})
  }, [])

  useEffect(() => {
    api.settings().then(setSettings).catch(() => {})
    api.info().then(setInfo).catch(() => {})
    api.logs().then((l) => setLogs(l.slice(-600))).catch(() => {})
    loadServers()
    // Conflict warnings clear once the user fixes the cause.
    const check = () => api.preflight().then((p) => setWarnings(p.warnings || [])).catch(() => {})
    check()
    const t = setInterval(check, 20000)
    return () => clearInterval(t)
  }, [loadServers])

  // Live events over SSE, with auto-reconnect.
  useEffect(() => {
    let stopped = false
    let retryTimer

    function connect() {
      const es = new EventSource('/api/events')
      esRef.current = es
      es.addEventListener('status', (e) => {
        setBackendUp(true)
        const st = JSON.parse(e.data)
        setStatus(st)
        setRateHistory((h) => {
          if (st.state !== 'connected') return st.state === 'connecting' ? [] : h
          const t = Date.now()
          // Status is pushed for many reasons; sample at most every ~1.5s.
          if (h.length && t - h[h.length - 1].t < 1500) return h
          return [...h, { t, rx: st.rxRate || 0, tx: st.txRate || 0 }].slice(-120)
        })
      })
      es.addEventListener('setup', (e) => setSetup(JSON.parse(e.data)))
      es.addEventListener('log', (e) => setLogs((l) => [...l, JSON.parse(e.data)].slice(-600)))
      es.addEventListener('servers', () => loadServers())
      es.onerror = () => {
        es.close()
        setBackendUp(false)
        if (!stopped) retryTimer = setTimeout(connect, 2000)
      }
    }
    connect()
    return () => {
      stopped = true
      clearTimeout(retryTimer)
      esRef.current?.close()
    }
  }, [loadServers])

  const saveSettings = async (cfg) => {
    const saved = await api.saveSettings(cfg)
    setSettings(saved)
    return saved
  }

  const quit = async () => {
    const msg = status.state === 'disconnected' ? 'Quit VPN Gate Client?' : 'Quitting will disconnect the VPN. Quit anyway?'
    if (!window.confirm(msg)) return
    setQuitting(true)
    await api.quit().catch(() => {})
  }

  if (quitting) return <Closed />

  const ready = setup?.phase === 'ready'
  const blocking = setupBlocking(setup)
  const fullHeight = page === 'servers' || page === 'activity'

  return (
    <div className="flex h-full">
      <Sidebar page={page} onNavigate={setPage} status={status} info={info} onQuit={quit} />

      <div className="flex min-w-0 flex-1 flex-col">
        <MobileNav page={page} onNavigate={setPage} status={status} />
        <div className="space-y-2 px-6 pt-4 empty:hidden">
          {!backendUp && (
            <Banner tone="error" icon={WifiOff}>
              Lost connection to the VPN service — reconnecting… <Spinner className="ml-1" />
            </Banner>
          )}
          {blocking && setupHidden && (
            <Banner tone="warn" icon={TriangleAlert}>
              {setup.message} — you can’t connect until setup is finished.
              <button className="ml-auto flex items-center gap-1 font-semibold text-amber-100 hover:underline" onClick={() => setSetupHidden(false)}>
                Finish setup <ArrowRight className="icon" />
              </button>
            </Banner>
          )}
          {warnings.map((w) => (
            <Banner key={w} tone="warn" icon={TriangleAlert}>{w}</Banner>
          ))}
        </div>

        <main className={`min-h-0 flex-1 px-6 py-5 ${fullHeight ? 'overflow-hidden' : 'overflow-y-auto'}`}>
          <div key={page} className={`mx-auto max-w-6xl animate-enter ${fullHeight ? 'h-full' : ''}`}>
            {page === 'dashboard' && (
              <Dashboard
                status={status}
                settings={settings}
                servers={servers}
                logs={logs}
                rateHistory={rateHistory}
                ready={ready}
                onSaveSettings={saveSettings}
                onNavigate={setPage}
              />
            )}
            {page === 'servers' && <Servers servers={servers} status={status} ready={ready} onReload={loadServers} />}
            {page === 'activity' && <Activity logs={logs} />}
            {page === 'settings' && settings && <Settings settings={settings} servers={servers} info={info} onSave={saveSettings} />}
          </div>
        </main>
      </div>

      {blocking && !setupHidden && <Setup setup={setup} info={info} onDismiss={() => setSetupHidden(true)} />}
    </div>
  )
}

function Sidebar({ page, onNavigate, status, info, onQuit }) {
  const st = stateStyle(status.state)
  return (
    <aside className="hidden w-60 shrink-0 flex-col border-r border-white/[0.05] bg-ink-900/70 md:flex">
      <div className="flex items-center gap-3 px-5 pt-5 pb-6">
        <Logo className="size-9" />
        <div className="leading-tight">
          <div className="text-[14px] font-semibold tracking-tight text-zinc-50">VPN Gate</div>
          <div className="text-[11px] text-zinc-500">Client {info?.version && info.version !== 'dev' ? info.version : ''}</div>
        </div>
      </div>

      <nav className="flex flex-col gap-0.5 px-3">
        {PAGES.map(({ id, name, Icon }) => (
          <button
            key={id}
            onClick={() => onNavigate(id)}
            className={`group relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-[13px] font-medium transition ${
              page === id ? 'bg-white/[0.06] text-zinc-50' : 'text-zinc-400 hover:bg-white/[0.03] hover:text-zinc-100'
            }`}
          >
            {page === id && <span className="absolute top-2 bottom-2 left-0 w-0.5 rounded-full bg-brand-500" />}
            <Icon className={`icon text-[17px] ${page === id ? 'text-brand-400' : 'text-zinc-500 group-hover:text-zinc-300'}`} />
            {name}
          </button>
        ))}
      </nav>

      <div className="mt-auto space-y-3 p-3">
        <button onClick={() => onNavigate('dashboard')} className="w-full rounded-xl bg-white/[0.03] p-3 text-left ring-1 ring-white/[0.05] transition hover:bg-white/[0.05]">
          <div className="flex items-center gap-2">
            <span className={`size-2 rounded-full ${st.dot}`} />
            <span className={`text-xs font-semibold ${st.text}`}>{st.label}</span>
          </div>
          {status.server && status.state !== 'disconnected' ? (
            <div className="mt-1.5 flex items-center gap-2 text-xs text-zinc-400">
              <Flag cc={status.server.countryCode} />
              <span className="truncate">{status.server.country}</span>
            </div>
          ) : (
            <div className="mt-1.5 text-xs text-zinc-500">Not connected</div>
          )}
        </button>
        {info?.canQuit && (
          <button onClick={onQuit} className="btn btn-ghost w-full justify-start text-zinc-500">
            <Power className="icon" /> Quit
          </button>
        )}
      </div>
    </aside>
  )
}

function MobileNav({ page, onNavigate, status }) {
  return (
    <div className="flex items-center gap-2 border-b border-white/[0.05] bg-ink-900/70 px-4 py-3 md:hidden">
      <Logo className="size-7" />
      <nav className="flex flex-1 gap-1 overflow-x-auto">
        {PAGES.map(({ id, name, Icon }) => (
          <button
            key={id}
            onClick={() => onNavigate(id)}
            className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium ${page === id ? 'bg-white/[0.07] text-zinc-50' : 'text-zinc-400'}`}
          >
            <Icon className="icon" /> <span className="hidden sm:inline">{name}</span>
          </button>
        ))}
      </nav>
      <StatusPill status={status} />
    </div>
  )
}

function Banner({ tone, icon: Icon, children }) {
  const cls = tone === 'error'
    ? 'border-rose-400/25 bg-rose-400/[0.08] text-rose-200'
    : 'border-amber-400/25 bg-amber-400/[0.08] text-amber-200'
  return (
    <div className={`mx-auto flex max-w-6xl animate-enter items-center gap-2.5 rounded-xl border px-4 py-2.5 text-[13px] ${cls}`}>
      <Icon className="icon shrink-0 text-base" />
      {children}
    </div>
  )
}

function Closed() {
  return (
    <div className="grid h-full place-items-center p-6">
      <div className="flex animate-pop flex-col items-center text-center">
        <Logo className="size-14" />
        <h2 className="mt-5 text-lg font-semibold text-zinc-50">VPN Gate Client has quit</h2>
        <p className="mt-1 text-[13px] text-zinc-500">You can close this window. Run the app again to reconnect.</p>
      </div>
    </div>
  )
}
