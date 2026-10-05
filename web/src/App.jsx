import React, { useEffect, useRef, useState, useCallback } from 'react'
import { ShieldCheck, LayoutDashboard, Server as ServerIcon, Settings as SettingsIcon, TriangleAlert } from 'lucide-react'
import { api } from './api.js'
import Dashboard from './components/Dashboard.jsx'
import Servers from './components/Servers.jsx'
import Settings from './components/Settings.jsx'
import { StatusPill } from './components/widgets.jsx'

const TABS = [
  { name: 'Dashboard', Icon: LayoutDashboard },
  { name: 'Servers', Icon: ServerIcon },
  { name: 'Settings', Icon: SettingsIcon },
]

export default function App() {
  const [tab, setTab] = useState('Dashboard')
  const [status, setStatus] = useState({ state: 'disconnected', message: 'connecting to backend…' })
  const [servers, setServers] = useState([])
  const [settings, setSettings] = useState(null)
  const [logs, setLogs] = useState([])
  const [preflight, setPreflight] = useState({ ok: true })
  const [backendUp, setBackendUp] = useState(true)
  const [rateHistory, setRateHistory] = useState([])
  const esRef = useRef(null)

  const loadServers = useCallback(() => {
    api.servers().then(setServers).catch(() => {})
  }, [])

  useEffect(() => {
    api.settings().then(setSettings).catch(() => {})
    api.logs().then((l) => setLogs(l.slice(-400))).catch(() => {})
    loadServers()
    // Preflight rechecks so warnings clear once the user fixes the cause.
    const check = () => api.preflight().then(setPreflight).catch(() => {})
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
          const next = [...h, { rx: st.rxRate || 0, tx: st.txRate || 0 }]
          return next.slice(-90)
        })
      })
      es.addEventListener('log', (e) => {
        setLogs((l) => [...l, JSON.parse(e.data)].slice(-400))
      })
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

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <span className="brand-icon"><ShieldCheck className="icon" /></span>
          <div>
            <h1>VPN Gate Client</h1>
            <span className="brand-sub">vpngate.net · {servers.length} servers</span>
          </div>
        </div>
        <nav className="tabs">
          {TABS.map(({ name, Icon }) => (
            <button key={name} className={`tab ${tab === name ? 'active' : ''}`} onClick={() => setTab(name)}>
              <Icon className="icon" /> {name}
            </button>
          ))}
        </nav>
        <StatusPill status={status} />
      </header>

      {!backendUp && (
        <div className="banner error"><TriangleAlert className="icon" /> Backend unreachable — retrying…</div>
      )}
      {!preflight.ok && (
        <div className="banner warn">
          <TriangleAlert className="icon" /> {preflight.error} — connections will fail until this is fixed.
        </div>
      )}
      {(preflight.warnings || []).map((w) => (
        <div key={w} className="banner warn"><TriangleAlert className="icon" /> {w}</div>
      ))}

      <main className="content">
        {tab === 'Dashboard' && (
          <Dashboard
            status={status}
            settings={settings}
            logs={logs}
            rateHistory={rateHistory}
            serverCount={servers.length}
            onGoSettings={() => setTab('Settings')}
          />
        )}
        {tab === 'Servers' && (
          <Servers servers={servers} status={status} onReload={loadServers} />
        )}
        {tab === 'Settings' && settings && (
          <Settings settings={settings} servers={servers} onSave={saveSettings} />
        )}
      </main>
    </div>
  )
}
