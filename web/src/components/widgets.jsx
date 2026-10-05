import React from 'react'
import { Globe, CircleCheck, Circle, TriangleAlert, CircleX } from 'lucide-react'
import Flags from 'country-flag-icons/react/3x2'

// Crisp inline SVG country flag. Only the flags actually shown get mounted;
// scale by setting font-size on the wrapper (width tracks the 3:2 ratio).
export function Flag({ cc, name, className = '' }) {
  const code = (cc || '').toUpperCase()
  const F = Flags[code]
  if (!F) return <Globe className={`icon flag flag-fallback ${className}`} />
  return <F className={`flag ${className}`} title={name || cc} aria-label={name || cc} />
}

export function StatusPill({ status }) {
  const cls = {
    connected: 'ok',
    connecting: 'busy',
    retrying: 'busy',
    disconnected: 'off',
  }[status.state] || 'off'
  return (
    <div className={`status-pill ${cls}`}>
      <span className="dot" />
      {status.state}
      {status.server && status.state !== 'disconnected' && (
        <span className="pill-server">
          <Flag cc={status.server.countryCode} /> {status.server.hostname}
        </span>
      )}
    </div>
  )
}

const HEALTH = {
  good: { label: 'Good', Icon: CircleCheck },
  new: { label: 'New', Icon: Circle },
  flaky: { label: 'Flaky', Icon: TriangleAlert },
  dead: { label: 'Dead', Icon: CircleX },
}

export function HealthBadge({ health }) {
  const { label, Icon } = HEALTH[health] || HEALTH.new
  return (
    <span className={`health health-${health}`}>
      <Icon className="icon" /> {label}
    </span>
  )
}

export function ScoreBar({ score }) {
  const pct = Math.max(0, Math.min(100, Math.round(score * 75)))
  return (
    <div className="scorebar" title={`score ${score}`}>
      <div className="scorebar-fill" style={{ width: `${pct}%` }} />
    </div>
  )
}

export function Sparkline({ data, color = 'var(--accent)', height = 44 }) {
  const w = 220
  if (!data || data.length < 2) {
    return <svg className="spark" viewBox={`0 0 ${w} ${height}`} width="100%" height={height} />
  }
  const max = Math.max(...data, 1)
  const pts = data
    .map((v, i) => {
      const x = (i / (data.length - 1)) * w
      const y = height - 3 - (v / max) * (height - 8)
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
  const areaPts = `0,${height} ${pts} ${w},${height}`
  return (
    <svg className="spark" viewBox={`0 0 ${w} ${height}`} width="100%" height={height} preserveAspectRatio="none">
      <polygon points={areaPts} fill={color} opacity="0.1" />
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.8" strokeLinejoin="round" />
    </svg>
  )
}
