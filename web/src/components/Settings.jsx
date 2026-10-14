import React, { useEffect, useRef, useState } from 'react'
import { Check, RefreshCw, Zap, Skull, Globe2, Info } from 'lucide-react'
import { Switch, Spinner, useToast } from './widgets.jsx'
import CountryPicker from './CountryPicker.jsx'

const ROTATE_CHOICES = [60, 120, 180, 360, 720, 1440]

// Settings save automatically (debounced) — no save button to forget.
export default function Settings({ settings, servers, info, onSave }) {
  const [cfg, setCfg] = useState(settings)
  const [state, setState] = useState('idle') // idle | saving | saved
  const toast = useToast()
  const timer = useRef(null)
  const first = useRef(true)

  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    clearTimeout(timer.current)
    setState('saving')
    timer.current = setTimeout(async () => {
      try {
        const saved = await onSave(cfg)
        // Reflect server-side clamping (e.g. rotation floor) in the form —
        // only when something changed, or this effect would save forever.
        const clamped = pickClamped(saved)
        setCfg((c) => (Object.keys(clamped).some((k) => c[k] !== clamped[k]) ? { ...c, ...clamped } : c))
        setState('saved')
      } catch (e) {
        toast(e.message)
        setState('idle')
      }
    }, 450)
    return () => clearTimeout(timer.current)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cfg])

  const set = (k, v) => setCfg((c) => ({ ...c, [k]: v }))
  const minRotate = info?.minRotate || 60
  const isWindows = info?.os === 'windows'

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div className="flex items-end justify-between">
        <div>
          <h2 className="text-lg font-semibold text-zinc-50">Settings</h2>
          <p className="text-xs text-zinc-500">Changes are saved automatically.</p>
        </div>
        <span className="flex h-6 items-center gap-1.5 text-xs text-zinc-500" aria-live="polite">
          {state === 'saving' && <><Spinner /> Saving…</>}
          {state === 'saved' && <span className="flex animate-fade items-center gap-1 text-emerald-300"><Check className="icon" /> Saved</span>}
        </span>
      </div>

      <Section icon={RefreshCw} title="Auto-rotate" desc="Periodically move to a fresh server while connected.">
        <Row label="Rotate servers automatically" hint="When the interval elapses, the app switches to a different healthy server in your chosen location.">
          <Switch checked={cfg.autoRotate} onChange={(v) => set('autoRotate', v)} label="Auto-rotate" />
        </Row>
        <div className={`transition-opacity ${cfg.autoRotate ? '' : 'pointer-events-none opacity-40'}`}>
          <div className="pt-1 pb-3">
            <div className="mb-2 text-[13px] font-medium text-zinc-200">Rotate every</div>
            <div className="grid grid-cols-3 gap-1.5 rounded-xl bg-ink-900 p-1 ring-1 ring-white/[0.06] sm:grid-cols-6">
              {ROTATE_CHOICES.filter((m) => m >= minRotate).map((m) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => set('rotateIntervalMin', m)}
                  className={`rounded-lg py-1.5 text-[13px] font-medium transition ${
                    cfg.rotateIntervalMin === m ? 'bg-brand-500 text-white shadow' : 'text-zinc-400 hover:bg-white/[0.05] hover:text-zinc-100'
                  }`}
                >
                  {m < 60 ? `${m}m` : `${m / 60}h`}
                </button>
              ))}
            </div>
            <p className="mt-2.5 flex gap-1.5 text-xs text-zinc-500">
              <Info className="icon mt-px shrink-0" />
              The minimum is 1 hour. VPN Gate servers are run by volunteers, so the app never hops faster than that.
              Each rotation briefly drops the tunnel while switching.
            </p>
          </div>
        </div>
      </Section>

      <Section icon={Zap} title="Connection" desc="How the app finds and keeps a working server.">
        <Row label="Connect on startup" hint="Start connecting as soon as the app launches.">
          <Switch checked={cfg.autoConnect} onChange={(v) => set('autoConnect', v)} label="Connect on startup" />
        </Row>
        <Row label="Auto-reconnect" hint="If the tunnel drops, immediately find another server.">
          <Switch checked={cfg.autoReconnect} onChange={(v) => set('autoReconnect', v)} label="Auto-reconnect" />
        </Row>
        <Row label="Quick reachability check" hint="Probe TCP servers briefly before a full connection attempt to skip dead ones.">
          <Switch checked={cfg.precheckTcp} onChange={(v) => set('precheckTcp', v)} label="Reachability check" />
        </Row>
        <Row
          label="Use VPN DNS"
          hint={isWindows
            ? 'OpenVPN applies the server’s DNS to the tunnel adapter automatically on Windows.'
            : 'Apply the VPN’s DNS servers via your system’s resolv.conf hooks (if installed) to avoid DNS leaks.'}
        >
          <Switch checked={cfg.manageDns} onChange={(v) => set('manageDns', v)} disabled={isWindows} label="Use VPN DNS" />
        </Row>
        <Row label="Connect timeout" hint="Give up on a server after this long.">
          <Num value={cfg.connectTimeoutSec} min={10} max={120} unit="sec" onChange={(v) => set('connectTimeoutSec', v)} />
        </Row>
        <Row label="Servers per round" hint="Candidates tried before refreshing the list and starting over.">
          <Num value={cfg.maxAutoCandidates} min={3} max={100} unit="servers" onChange={(v) => set('maxAutoCandidates', v)} />
        </Row>
      </Section>

      <Section icon={Globe2} title="Preferred location" desc="Auto-connect only uses servers in these countries. None selected means any country.">
        <div className="pt-1 pb-2">
          <CountryPicker servers={servers} value={cfg.countries} onChange={(v) => set('countries', v)} />
        </div>
      </Section>

      <Section icon={Skull} title="Dead servers" desc="Servers that keep failing are skipped for a while.">
        <Row label="Mark dead after" hint="Consecutive failures before a server is deprioritized.">
          <Num value={cfg.deadThreshold} min={1} max={10} unit="fails" onChange={(v) => set('deadThreshold', v)} />
        </Row>
        <Row label="Cooldown" hint="How long a dead server is skipped before it may be retried.">
          <Num value={cfg.deadCooldownMin} min={5} max={2880} unit="min" onChange={(v) => set('deadCooldownMin', v)} />
        </Row>
      </Section>

      {info && (
        <p className="pb-4 text-center text-xs text-zinc-600">
          VPN Gate Client {info.version} · {info.os}/{info.arch} · data in <span className="font-mono">{info.dataDir}</span>
        </p>
      )}
    </div>
  )
}

function pickClamped(s) {
  const out = {}
  for (const k of ['rotateIntervalMin', 'connectTimeoutSec', 'deadThreshold', 'deadCooldownMin', 'maxAutoCandidates']) out[k] = s[k]
  return out
}

function Section({ icon: Icon, title, desc, children }) {
  return (
    <section className="card p-5">
      <div className="mb-2 flex items-start gap-3">
        <span className="grid size-8 place-items-center rounded-lg bg-brand-500/10 text-brand-400 ring-1 ring-brand-500/20">
          <Icon className="icon" />
        </span>
        <div>
          <h3 className="text-sm font-semibold text-zinc-100">{title}</h3>
          <p className="text-xs text-zinc-500">{desc}</p>
        </div>
      </div>
      <div className="divide-y divide-white/[0.05] pl-11">{children}</div>
    </section>
  )
}

function Row({ label, hint, children }) {
  return (
    <div className="flex items-center justify-between gap-6 py-3">
      <div>
        <div className="text-[13px] font-medium text-zinc-200">{label}</div>
        {hint && <div className="mt-0.5 text-xs text-zinc-500">{hint}</div>}
      </div>
      {children}
    </div>
  )
}

// Number field that only commits valid values (so typing doesn't thrash saves).
function Num({ value, min, max, unit, onChange }) {
  const [text, setText] = useState(String(value))
  useEffect(() => setText(String(value)), [value])
  const commit = () => {
    const n = Math.round(Number(text))
    if (Number.isFinite(n)) {
      const v = Math.min(max, Math.max(min, n))
      setText(String(v))
      if (v !== value) onChange(v)
    } else setText(String(value))
  }
  return (
    <span className="flex shrink-0 items-center gap-2">
      <input
        type="number"
        className="input w-20 text-right tabular-nums"
        min={min}
        max={max}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
      />
      <span className="w-12 text-xs text-zinc-500">{unit}</span>
    </span>
  )
}
