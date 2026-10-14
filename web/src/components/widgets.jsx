import React, { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Globe, CircleCheck, Circle, TriangleAlert, CircleX, X, Info } from 'lucide-react'
import Flags from 'country-flag-icons/react/3x2'

// Crisp inline SVG country flag; scale with font-size (width tracks 3:2).
export function Flag({ cc, name, className = '' }) {
  const code = (cc || '').toUpperCase()
  const F = Flags[code]
  if (!F) return <Globe className={`icon text-zinc-500 ${className}`} />
  return <F className={`flag ${className}`} title={name || cc} aria-label={name || cc} />
}

export function Logo({ className = 'size-8' }) {
  return <img src="/logo.svg" alt="" className={`${className} rounded-[22%]`} draggable="false" />
}

const STATE = {
  connected: { label: 'Protected', dot: 'bg-emerald-400', text: 'text-emerald-300', ring: 'ring-emerald-400/25 bg-emerald-400/10' },
  connecting: { label: 'Connecting', dot: 'bg-amber-400 animate-pulse', text: 'text-amber-200', ring: 'ring-amber-400/25 bg-amber-400/10' },
  retrying: { label: 'Retrying', dot: 'bg-amber-400 animate-pulse', text: 'text-amber-200', ring: 'ring-amber-400/25 bg-amber-400/10' },
  disconnected: { label: 'Unprotected', dot: 'bg-rose-400', text: 'text-rose-200', ring: 'ring-rose-400/20 bg-rose-400/[0.08]' },
}

export function stateStyle(state) {
  return STATE[state] || STATE.disconnected
}

export function StatusPill({ status }) {
  const st = stateStyle(status.state)
  return (
    <span className={`inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-semibold ring-1 ${st.ring} ${st.text}`}>
      <span className={`size-1.5 rounded-full ${st.dot}`} />
      {st.label}
    </span>
  )
}

const HEALTH = {
  good: { label: 'Good', Icon: CircleCheck, cls: 'text-emerald-300 bg-emerald-400/10 ring-emerald-400/20' },
  new: { label: 'New', Icon: Circle, cls: 'text-zinc-400 bg-white/[0.04] ring-white/10' },
  flaky: { label: 'Flaky', Icon: TriangleAlert, cls: 'text-amber-200 bg-amber-400/10 ring-amber-400/20' },
  dead: { label: 'Dead', Icon: CircleX, cls: 'text-rose-300 bg-rose-400/10 ring-rose-400/20' },
}

export function HealthBadge({ health }) {
  const { label, Icon, cls } = HEALTH[health] || HEALTH.new
  return (
    <span className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-semibold ring-1 ${cls}`}>
      <Icon className="icon text-[12px]" /> {label}
    </span>
  )
}

// Meter: brand fill on a lighter step of the same hue.
export function ScoreBar({ score }) {
  const pct = Math.max(4, Math.min(100, Math.round(score * 75)))
  return (
    <div className="h-1.5 w-16 overflow-hidden rounded-full bg-brand-500/15" title={`score ${score}`}>
      <div className="h-full rounded-full bg-brand-500 transition-[width] duration-500" style={{ width: `${pct}%` }} />
    </div>
  )
}

export function Switch({ checked, onChange, disabled, label }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={!!checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border transition-colors duration-200 disabled:opacity-40 ${
        checked ? 'border-brand-500 bg-brand-500' : 'border-white/10 bg-ink-900'
      }`}
    >
      <span
        className={`inline-block size-4.5 rounded-full shadow-sm transition-transform duration-200 ease-out ${
          checked ? 'translate-x-[22px] bg-white' : 'translate-x-[3px] bg-zinc-500'
        }`}
      />
    </button>
  )
}

export function Spinner({ className = '' }) {
  return (
    <svg className={`icon animate-spin ${className}`} viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeOpacity=".2" strokeWidth="3" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  )
}

export function Empty({ icon: Icon = Info, title, children }) {
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-12 text-center">
      <span className="grid size-11 place-items-center rounded-2xl bg-white/[0.04] text-xl text-zinc-500 ring-1 ring-white/[0.06]">
        <Icon className="icon" />
      </span>
      <p className="font-medium text-zinc-300">{title}</p>
      {children && <p className="max-w-sm text-xs text-zinc-500">{children}</p>}
    </div>
  )
}

// ---- toasts ----

const ToastCtx = createContext(() => {})
export const useToast = () => useContext(ToastCtx)

export function ToastProvider({ children }) {
  const [items, setItems] = useState([])
  const id = useRef(0)
  const dismiss = useCallback((k) => setItems((xs) => xs.filter((x) => x.k !== k)), [])
  const push = useCallback(
    (message, tone = 'error') => {
      const k = ++id.current
      setItems((xs) => [...xs.slice(-3), { k, message, tone }])
      setTimeout(() => dismiss(k), tone === 'error' ? 6000 : 3500)
    },
    [dismiss],
  )
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed right-5 bottom-5 z-50 flex w-80 flex-col gap-2">
        {items.map((t) => (
          <div
            key={t.k}
            role="status"
            className={`pointer-events-auto flex animate-toast items-start gap-2.5 rounded-xl border bg-ink-750/95 p-3 pr-2 text-[13px] shadow-2xl backdrop-blur ${
              t.tone === 'error' ? 'border-rose-400/25' : 'border-emerald-400/25'
            }`}
          >
            {t.tone === 'error' ? (
              <CircleX className="icon mt-0.5 text-base text-rose-400" />
            ) : (
              <CircleCheck className="icon mt-0.5 text-base text-emerald-400" />
            )}
            <span className="flex-1 text-zinc-200">{t.message}</span>
            <button className="rounded-md p-0.5 text-zinc-500 hover:text-zinc-200" onClick={() => dismiss(t.k)} aria-label="Dismiss">
              <X className="icon" />
            </button>
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  )
}

// Runs an async action, routing failures to a toast; returns [run, busy].
export function useAction() {
  const toast = useToast()
  const [busy, setBusy] = useState('')
  const run = useCallback(
    (key, fn) => async () => {
      setBusy(key)
      try {
        await fn()
      } catch (e) {
        toast(e.message)
      } finally {
        setBusy('')
      }
    },
    [toast],
  )
  return [run, busy]
}

// Re-render every `ms` while active (for live clocks).
export function useNow(active, ms = 1000) {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    if (!active) return
    setNow(Date.now())
    const t = setInterval(() => setNow(Date.now()), ms)
    return () => clearInterval(t)
  }, [active, ms])
  return now
}
