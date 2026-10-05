import React, { useMemo, useState } from 'react'
import { Check } from 'lucide-react'
import { Flag } from './widgets.jsx'

export default function Settings({ settings, servers, onSave }) {
  const [cfg, setCfg] = useState(settings)
  const [saved, setSaved] = useState(false)
  const [err, setErr] = useState('')

  const countries = useMemo(() => {
    const m = new Map()
    for (const s of servers) m.set(s.countryCode, s.country)
    return [...m.entries()].sort((a, b) => a[1].localeCompare(b[1]))
  }, [servers])

  const set = (k, v) => {
    setCfg((c) => ({ ...c, [k]: v }))
    setSaved(false)
  }

  const toggleCountry = (code) => {
    set(
      'countries',
      cfg.countries.includes(code)
        ? cfg.countries.filter((c) => c !== code)
        : [...cfg.countries, code],
    )
  }

  const save = async () => {
    setErr('')
    try {
      await onSave(cfg)
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (e) {
      setErr(e.message)
    }
  }

  const Toggle = ({ k, label, hint }) => (
    <label className="setting">
      <div>
        <strong>{label}</strong>
        {hint && <div className="dim small">{hint}</div>}
      </div>
      <input type="checkbox" className="switch" checked={!!cfg[k]} onChange={(e) => set(k, e.target.checked)} />
    </label>
  )

  const Num = ({ k, label, hint, min, max, unit }) => (
    <label className="setting">
      <div>
        <strong>{label}</strong>
        {hint && <div className="dim small">{hint}</div>}
      </div>
      <span className="num-input">
        <input
          type="number"
          className="input narrow"
          min={min}
          max={max}
          value={cfg[k]}
          onChange={(e) => set(k, Number(e.target.value))}
        />
        <span className="dim small">{unit}</span>
      </span>
    </label>
  )

  return (
    <div className="settings">
      <section className="card">
        <h3>Connection behaviour</h3>
        <Toggle k="autoConnect" label="Auto-connect on startup" hint="Start connecting as soon as the app launches" />
        <Toggle k="autoReconnect" label="Auto-reconnect" hint="If the tunnel drops, immediately hunt for a new server" />
        <Toggle k="precheckTcp" label="TCP pre-check" hint="Quickly probe TCP servers before a full OpenVPN attempt — skips obviously dead ones" />
        <Toggle k="manageDns" label="Manage DNS" hint="Use the system's resolv.conf update hooks (if installed) to apply the VPN's DNS and avoid leaks" />
        <Num k="connectTimeoutSec" label="Connect timeout" hint="Give up on a server after this long" min={10} max={120} unit="sec" />
        <Num k="maxAutoCandidates" label="Servers per round" hint="How many candidates auto-connect tries before refreshing the list" min={3} max={100} unit="servers" />
      </section>

      <section className="card">
        <h3>Dead server handling</h3>
        <Num k="deadThreshold" label="Dead after" hint="Consecutive failures before a server is marked dead and deprioritized" min={1} max={10} unit="fails" />
        <Num k="deadCooldownMin" label="Cooldown" hint="How long a dead server is skipped before it may be retried" min={5} max={2880} unit="min" />
      </section>

      <section className="card">
        <h3>Preferred countries</h3>
        <p className="dim small">
          Auto-connect only tries servers in the selected countries. Select none to allow all.
        </p>
        <div className="chips">
          {countries.map(([code, name]) => (
            <button
              key={code}
              className={`chip ${cfg.countries.includes(code) ? 'on' : ''}`}
              onClick={() => toggleCountry(code)}
            >
              <Flag cc={code} /> {name}
            </button>
          ))}
          {countries.length === 0 && <span className="dim">Load the server list first.</span>}
        </div>
      </section>

      <div className="save-row">
        {err && <span className="error-text">{err}</span>}
        {saved && <span className="ok-text"><Check className="icon" /> Saved</span>}
        <button className="btn primary" onClick={save}>Save settings</button>
      </div>
    </div>
  )
}
