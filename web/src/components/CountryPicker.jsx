import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Check, ChevronDown, Globe, Search } from 'lucide-react'
import { Flag } from './widgets.jsx'

// Countries present in the server list, most servers first.
export function useCountries(servers) {
  return useMemo(() => {
    const m = new Map()
    for (const s of servers) {
      const cur = m.get(s.countryCode) || { code: s.countryCode, name: s.country, n: 0 }
      cur.n++
      m.set(s.countryCode, cur)
    }
    return [...m.values()].sort((a, b) => b.n - a.n || a.name.localeCompare(b.name))
  }, [servers])
}

// Multi-select country chooser in a popover. Empty selection = any country.
export default function CountryPicker({ servers, value, onChange, align = 'left' }) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const ref = useRef(null)
  const countries = useCountries(servers)
  const names = useMemo(() => Object.fromEntries(countries.map((c) => [c.code, c.name])), [countries])

  useEffect(() => {
    if (!open) return
    const onDown = (e) => ref.current && !ref.current.contains(e.target) && setOpen(false)
    const onKey = (e) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const shown = countries.filter((c) => !q || c.name.toLowerCase().includes(q.toLowerCase()) || c.code.toLowerCase() === q.toLowerCase())
  const toggle = (code) => onChange(value.includes(code) ? value.filter((c) => c !== code) : [...value, code])

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2.5 rounded-xl border border-white/[0.08] bg-ink-900/80 px-3 py-2.5 text-left transition hover:border-white/15"
        aria-expanded={open}
      >
        {value.length === 0 ? (
          <>
            <span className="grid size-6 place-items-center rounded-md bg-brand-500/15 text-brand-400">
              <Globe className="icon" />
            </span>
            <span className="flex-1">
              <span className="block text-[13px] font-medium text-zinc-100">Fastest available</span>
              <span className="block text-[11px] text-zinc-500">Any country</span>
            </span>
          </>
        ) : (
          <>
            <span className="flex -space-x-1.5 text-lg">
              {value.slice(0, 3).map((c) => (
                <Flag key={c} cc={c} className="ring-2 ring-ink-900" />
              ))}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-[13px] font-medium text-zinc-100">
                {value.length === 1 ? names[value[0]] || value[0] : `${value.length} countries`}
              </span>
              <span className="block truncate text-[11px] text-zinc-500">
                {value.length === 1 ? 'Best server in this country' : value.map((c) => names[c] || c).join(', ')}
              </span>
            </span>
          </>
        )}
        <ChevronDown className={`icon text-zinc-500 transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>

      {open && (
        <div
          className={`absolute z-40 mt-2 w-80 animate-pop overflow-hidden rounded-2xl border border-white/10 bg-ink-750/95 shadow-2xl backdrop-blur-xl ${
            align === 'right' ? 'right-0' : 'left-0'
          }`}
        >
          <div className="border-b border-white/[0.06] p-2">
            <label className="flex items-center gap-2 rounded-lg bg-ink-900 px-2.5">
              <Search className="icon text-zinc-500" />
              <input
                autoFocus
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="Search countries…"
                className="w-full bg-transparent py-2 text-[13px] outline-none placeholder:text-zinc-500"
              />
            </label>
          </div>
          <div className="max-h-72 overflow-y-auto p-1.5">
            <Row active={value.length === 0} onClick={() => onChange([])}>
              <Globe className="icon text-base text-brand-400" />
              <span className="flex-1">Any country</span>
            </Row>
            {shown.map((c) => (
              <Row key={c.code} active={value.includes(c.code)} onClick={() => toggle(c.code)}>
                <Flag cc={c.code} className="text-base" />
                <span className="flex-1 truncate">{c.name}</span>
                <span className="text-[11px] tabular-nums text-zinc-500">{c.n}</span>
              </Row>
            ))}
            {shown.length === 0 && <p className="px-3 py-6 text-center text-xs text-zinc-500">No countries match.</p>}
          </div>
          {value.length > 0 && (
            <div className="flex items-center justify-between border-t border-white/[0.06] px-3 py-2 text-xs text-zinc-500">
              {value.length} selected
              <button className="text-brand-400 hover:text-brand-500" onClick={() => onChange([])}>
                Clear
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function Row({ active, onClick, children }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[13px] transition ${
        active ? 'bg-brand-500/15 text-zinc-50' : 'text-zinc-300 hover:bg-white/[0.05]'
      }`}
    >
      {children}
      <Check className={`icon text-brand-400 ${active ? 'opacity-100' : 'opacity-0'}`} />
    </button>
  )
}
