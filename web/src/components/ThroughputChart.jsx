import React, { useEffect, useMemo, useRef, useState } from 'react'
import { fmtRate, fmtRateAxis } from '../api.js'

const SERIES = [
  { key: 'rx', label: 'Download', color: 'var(--color-series-down)' },
  { key: 'tx', label: 'Upload', color: 'var(--color-series-up)' },
]
const H = 170
const PAD = { top: 12, right: 14, bottom: 22, left: 40 }

// Clean tick step (1/2/5 × 10^n) covering max in ~3 steps.
function niceMax(max) {
  if (max <= 0) return 1
  const raw = max / 3
  const pow = 10 ** Math.floor(Math.log10(raw))
  const step = [1, 2, 5, 10].find((m) => m * pow >= raw) * pow
  return { step, top: Math.ceil(max / step) * step }
}

// Live download/upload rate: two lines over a shared time axis, one y-scale,
// legend + crosshair tooltip listing both series.
export default function ThroughputChart({ history }) {
  const wrap = useRef(null)
  const [w, setW] = useState(600)
  const [hover, setHover] = useState(null)

  useEffect(() => {
    const el = wrap.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setW(Math.max(260, e.contentRect.width)))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const data = history.length ? history : [{ t: Date.now(), rx: 0, tx: 0 }]
  const { step, top } = niceMax(Math.max(...data.map((d) => Math.max(d.rx, d.tx)), 1250))
  const t0 = data[0].t
  const t1 = Math.max(data[data.length - 1].t, t0 + 1000)
  const iw = w - PAD.left - PAD.right
  const ih = H - PAD.top - PAD.bottom
  const x = (t) => PAD.left + ((t - t0) / (t1 - t0)) * iw
  const y = (v) => PAD.top + ih - (v / top) * ih

  const paths = useMemo(
    () =>
      SERIES.map((s) => {
        const pts = data.map((d) => `${x(d.t).toFixed(1)},${y(d[s.key]).toFixed(1)}`)
        return {
          ...s,
          line: `M${pts.join('L')}`,
          area: `M${x(t0)},${y(0)}L${pts.join('L')}L${x(data[data.length - 1].t)},${y(0)}Z`,
        }
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [data, w, top],
  )

  const ticks = []
  for (let v = 0; v <= top + 1e-9; v += step) ticks.push(v)
  const last = data[data.length - 1]
  const spanSec = Math.round((t1 - t0) / 1000)

  const onMove = (e) => {
    const r = e.currentTarget.getBoundingClientRect()
    const px = e.clientX - r.left
    let best = 0
    for (let i = 1; i < data.length; i++) if (Math.abs(x(data[i].t) - px) < Math.abs(x(data[best].t) - px)) best = i
    setHover(best)
  }
  const hp = hover != null ? data[hover] : null

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold text-zinc-100">Throughput</h3>
        <div className="flex items-center gap-4 text-xs text-zinc-400">
          {SERIES.map((s) => (
            <span key={s.key} className="flex items-center gap-1.5">
              <span className="h-0.5 w-3.5 rounded-full" style={{ background: s.color }} />
              {s.label}
            </span>
          ))}
        </div>
      </div>
      <div ref={wrap} className="relative" onPointerMove={onMove} onPointerLeave={() => setHover(null)}>
        <svg width={w} height={H} className="block select-none" role="img" aria-label={`Throughput: download ${fmtRate(last.rx)}, upload ${fmtRate(last.tx)}`}>
          {ticks.map((v) => (
            <g key={v}>
              <line x1={PAD.left} x2={w - PAD.right} y1={y(v)} y2={y(v)} stroke="rgb(255 255 255 / .06)" strokeWidth="1" />
              <text x={PAD.left - 8} y={y(v)} dy="0.32em" textAnchor="end" className="fill-zinc-500 text-[10px] tabular-nums">
                {fmtRateAxis(v)}
              </text>
            </g>
          ))}
          <text x={PAD.left} y={H - 4} className="fill-zinc-500 text-[10px]">
            {spanSec > 5 ? `−${spanSec >= 60 ? `${Math.round(spanSec / 60)}m` : `${spanSec}s`}` : ''}
          </text>
          <text x={w - PAD.right} y={H - 4} textAnchor="end" className="fill-zinc-500 text-[10px]">
            now
          </text>
          {paths.map((p) => (
            <path key={`a${p.key}`} d={p.area} fill={p.color} opacity="0.1" />
          ))}
          {paths.map((p) => (
            <path key={`l${p.key}`} d={p.line} fill="none" stroke={p.color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />
          ))}
          {hp && <line x1={x(hp.t)} x2={x(hp.t)} y1={PAD.top} y2={PAD.top + ih} stroke="rgb(255 255 255 / .25)" strokeWidth="1" />}
          {SERIES.map((s) => {
            const d = hp || last
            return <circle key={s.key} cx={x(d.t)} cy={y(d[s.key])} r="4" fill={s.color} stroke="var(--color-ink-800)" strokeWidth="2" />
          })}
        </svg>
        {hp && (
          <div
            className="pointer-events-none absolute top-1 z-10 min-w-36 rounded-lg border border-white/10 bg-ink-700/95 px-2.5 py-2 text-xs shadow-xl backdrop-blur"
            style={x(hp.t) > w / 2 ? { right: w - x(hp.t) + 10 } : { left: x(hp.t) + 10 }}
          >
            <div className="mb-1 text-[10px] text-zinc-500">{new Date(hp.t).toLocaleTimeString()}</div>
            {SERIES.map((s) => (
              <div key={s.key} className="flex items-center gap-2">
                <span className="size-2 rounded-full" style={{ background: s.color }} />
                <span className="font-semibold text-zinc-100 tabular-nums">{fmtRate(hp[s.key])}</span>
                <span className="text-zinc-500">{s.label}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
