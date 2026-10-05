import React, { useEffect, useRef, useState } from 'react'
import { Lock, LockOpen, LoaderCircle, Zap, Power, ArrowDown, ArrowUp, Clock } from 'lucide-react'
import { api, fmtBytes, fmtRate, fmtDuration, timeAgo } from '../api.js'
import { Sparkline, HealthBadge, Flag } from './widgets.jsx'

export default function Dashboard({ status, settings, logs, rateHistory, serverCount, onGoSettings }) {
  const [recs, setRecs] = useState([])
  const [showLogs, setShowLogs] = useState(false)
  const [busy, setBusy] = useState(false)
  const [now, setNow] = useState(Date.now())
  const logEndRef = useRef(null)

  const countries = settings?.countries || []

  useEffect(() => {
    const load = () => api.recommendations(countries).then(setRecs).catch(() => {})
    load()
    const t = setInterval(load, 60000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(countries), status.state, serverCount])

  useEffect(() => {
    if (status.state !== 'connected') return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [status.state])

  useEffect(() => {
    if (showLogs) logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs, showLogs])

  const act = (fn) => async () => {
    setBusy(true)
    try {
      await fn()
    } catch (e) {
      alert(e.message)
    } finally {
      setBusy(false)
    }
  }

  const s = status.server
  const uptime = status.connectedAt ? now - new Date(status.connectedAt).getTime() : 0
  const isIdle = status.state === 'disconnected'
  const isBusy = status.state === 'connecting' || status.state === 'retrying'

  return (
    <div className="dashboard">
      <section className={`hero card state-${status.state}`}>
        <div className="hero-main">
          <div className={`orb ${status.state}`}>
            {status.state === 'connected' ? <Lock className="icon" />
              : isBusy ? <LoaderCircle className="icon" />
              : <LockOpen className="icon" />}
          </div>
          <div className="hero-info">
            <h2 className="hero-state">
              {status.state === 'connected' && 'Connected'}
              {status.state === 'connecting' && `Connecting${status.attempt ? ` · ${status.attempt}/${status.total}` : ''}…`}
              {status.state === 'retrying' && 'Retrying…'}
              {isIdle && 'Not connected'}
            </h2>
            <p className="hero-msg">{status.message}</p>
            {s && !isIdle && (
              <div className="hero-server">
                <Flag cc={s.countryCode} />
                <div>
                  <strong>{s.hostname}</strong> · {s.country}
                  <div className="mono dim">
                    {s.ip}:{s.port}/{s.proto}
                    {status.publicIp && <> → exit IP <strong>{status.publicIp}</strong></>}
                  </div>
                </div>
              </div>
            )}
            {status.lastError && isIdle && <p className="dim small">last error: {status.lastError}</p>}
          </div>
        </div>
        <div className="hero-actions">
          {isIdle ? (
            <button className="btn primary big" disabled={busy} onClick={act(() => api.auto(countries))}>
              <Zap className="icon" /> Auto Connect
            </button>
          ) : (
            <button className="btn danger big" disabled={busy} onClick={act(() => api.disconnect())}>
              <Power className="icon" /> {status.state === 'connected' ? 'Disconnect' : 'Cancel'}
            </button>
          )}
          <div className="country-hint">
            {countries.length ? (
              <>
                trying: {countries.map((c) => (
                  <span key={c} className="chip small"><Flag cc={c} /> {c}</span>
                ))}
              </>
            ) : (
              <span className="dim">any country</span>
            )}
            <button className="link" onClick={onGoSettings}>change</button>
          </div>
        </div>
      </section>

      {status.state === 'connected' && (
        <section className="stats-row">
          <div className="card stat">
            <span className="stat-label"><ArrowDown className="icon" /> Download</span>
            <span className="stat-value">{fmtRate(status.rxRate)}</span>
            <Sparkline data={rateHistory.map((r) => r.rx)} color="var(--ok)" />
            <span className="dim small">total {fmtBytes(status.rxBytes)}</span>
          </div>
          <div className="card stat">
            <span className="stat-label"><ArrowUp className="icon" /> Upload</span>
            <span className="stat-value">{fmtRate(status.txRate)}</span>
            <Sparkline data={rateHistory.map((r) => r.tx)} color="var(--accent)" />
            <span className="dim small">total {fmtBytes(status.txBytes)}</span>
          </div>
          <div className="card stat">
            <span className="stat-label"><Clock className="icon" /> Uptime</span>
            <span className="stat-value">{fmtDuration(uptime)}</span>
            <span className="dim small">since {new Date(status.connectedAt).toLocaleTimeString()}</span>
          </div>
        </section>
      )}

      <section className="card">
        <div className="card-head">
          <h3>Recommended servers</h3>
          <span className="dim small">ranked by speed, reliability & your history</span>
        </div>
        {recs.length === 0 ? (
          <p className="dim">No recommendations yet — refresh the server list from the Servers tab.</p>
        ) : (
          <div className="rec-list">
            {recs.map((r) => (
              <div key={r.id} className="rec">
                <Flag cc={r.countryCode} />
                <div className="rec-info">
                  <strong>{r.hostname}</strong>
                  <span className="dim small">
                    {r.country} · {r.speedMbps} Mbps · ping {r.pingMs || '—'}ms
                    {r.stats.successes > 0 && ` · worked ${timeAgo(r.stats.lastSuccess)}`}
                  </span>
                </div>
                <HealthBadge health={r.health} />
                <button
                  className="btn small"
                  disabled={busy || isBusy}
                  onClick={act(() => api.connect(r.id))}
                >
                  Connect
                </button>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="card">
        <div className="card-head">
          <h3>Activity log</h3>
          <button className="link" onClick={() => setShowLogs((v) => !v)}>
            {showLogs ? 'hide' : `show (${logs.length})`}
          </button>
        </div>
        {showLogs && (
          <div className="logbox">
            {logs.map((l, i) => (
              <div key={i} className={`logline ${l.line.startsWith('[app]') ? 'app' : ''}`}>
                <span className="dim">{new Date(l.time).toLocaleTimeString()}</span> {l.line}
              </div>
            ))}
            <div ref={logEndRef} />
          </div>
        )}
      </section>
    </div>
  )
}
