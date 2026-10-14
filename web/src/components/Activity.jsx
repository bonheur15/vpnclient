import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Search, Copy, ArrowDownToLine, ScrollText } from 'lucide-react'
import { Empty, useToast } from './widgets.jsx'

const FILTERS = [
  ['all', 'All'],
  ['app', 'App'],
  ['ovpn', 'OpenVPN'],
  ['problems', 'Problems'],
]

const PROBLEM = /error|fail|fatal|warning|lost|unreachable|refused|reset/i

export default function Activity({ logs }) {
  const [filter, setFilter] = useState('all')
  const [q, setQ] = useState('')
  const [follow, setFollow] = useState(true)
  const box = useRef(null)
  const toast = useToast()

  const rows = useMemo(() => {
    let out = logs
    if (filter === 'app') out = out.filter((l) => l.line.startsWith('[app]'))
    if (filter === 'ovpn') out = out.filter((l) => l.line.startsWith('[ovpn]'))
    if (filter === 'problems') out = out.filter((l) => PROBLEM.test(l.line))
    if (q) out = out.filter((l) => l.line.toLowerCase().includes(q.toLowerCase()))
    return out
  }, [logs, filter, q])

  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [rows, follow])

  const onScroll = (e) => {
    const el = e.currentTarget
    setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 40)
  }

  const copy = () => {
    const text = rows.map((l) => `${new Date(l.time).toISOString()} ${l.line}`).join('\n')
    navigator.clipboard?.writeText(text).then(() => toast(`Copied ${rows.length} lines`, 'ok'))
  }

  return (
    <div className="flex h-full flex-col gap-4">
      <div className="card flex flex-wrap items-center gap-2.5 p-3">
        <div className="flex gap-1 rounded-xl bg-ink-900 p-1 ring-1 ring-white/[0.06]">
          {FILTERS.map(([k, label]) => (
            <button
              key={k}
              onClick={() => setFilter(k)}
              className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                filter === k ? 'bg-ink-600 text-zinc-50 shadow' : 'text-zinc-400 hover:text-zinc-100'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
        <label className="flex min-w-48 flex-1 items-center gap-2 rounded-xl border border-white/[0.08] bg-ink-900 px-3">
          <Search className="icon text-zinc-500" />
          <input
            className="w-full bg-transparent py-2 text-[13px] outline-none placeholder:text-zinc-500"
            placeholder="Filter log…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </label>
        <button className="btn" onClick={copy} disabled={rows.length === 0}>
          <Copy className="icon" /> Copy
        </button>
      </div>

      <div className="card relative min-h-0 flex-1 overflow-hidden">
        <div ref={box} onScroll={onScroll} className="h-full overflow-y-auto px-4 py-3 font-mono text-[11.5px] leading-[1.8]">
          {rows.length === 0 && <Empty icon={ScrollText} title="No log lines" />}
          {rows.map((l, i) => {
            const app = l.line.startsWith('[app]')
            const bad = PROBLEM.test(l.line)
            return (
              <div key={i} className="flex gap-3 rounded px-1 hover:bg-white/[0.02]">
                <span className="shrink-0 text-zinc-600 tabular-nums">{new Date(l.time).toLocaleTimeString()}</span>
                <span className={`break-all whitespace-pre-wrap ${bad ? 'text-rose-300' : app ? 'text-sky-200' : 'text-zinc-400'}`}>
                  {l.line}
                </span>
              </div>
            )
          })}
        </div>
        {!follow && (
          <button
            className="btn btn-primary btn-sm absolute right-4 bottom-4 animate-pop shadow-xl"
            onClick={() => setFollow(true)}
          >
            <ArrowDownToLine className="icon" /> Latest
          </button>
        )}
      </div>
    </div>
  )
}
