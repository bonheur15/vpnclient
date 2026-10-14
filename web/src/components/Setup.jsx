import React from 'react'
import { Download, ShieldCheck, KeyRound, CircleCheck, TriangleAlert, ArrowRight } from 'lucide-react'
import { api } from '../api.js'
import { Logo, Spinner, useAction } from './widgets.jsx'

const WORKING = ['downloading', 'installing', 'checking']

// Is this setup state something the user should see a screen for?
export function setupBlocking(setup) {
  return setup && setup.phase !== 'ready' && setup.phase !== 'checking'
}

// First-run screen: shows automatic download/install progress, or the single
// action needed to finish (install, grant access, retry).
export default function Setup({ setup, info, onDismiss }) {
  const [run, busy] = useAction()
  const working = WORKING.includes(setup.phase)
  const steps = stepsFor(setup, info)

  return (
    <div className="fixed inset-0 z-40 grid animate-fade place-items-center bg-ink-950/80 p-6 backdrop-blur-md">
      <div className="card w-full max-w-lg animate-pop overflow-hidden p-0">
        <div className="relative px-7 pt-7 pb-5">
          <div className="pointer-events-none absolute -top-24 left-1/2 h-48 w-96 -translate-x-1/2 rounded-full bg-radial from-brand-500/25 to-transparent to-70%" />
          <Logo className="relative size-12" />
          <h2 className="relative mt-4 text-lg font-semibold text-zinc-50">
            {working ? 'Getting things ready' : setup.phase === 'error' ? 'Setup needs attention' : 'One more step'}
          </h2>
          <p className="relative mt-1 text-[13px] text-zinc-400">
            {working
              ? 'The app is setting up everything it needs. This only happens once.'
              : 'The app needs one thing before it can create a VPN tunnel.'}
          </p>
        </div>

        <ol className="space-y-1 px-5">
          {steps.map((s) => (
            <li key={s.label} className={`flex items-center gap-3 rounded-xl px-2.5 py-2.5 ${s.state === 'active' ? 'bg-white/[0.04]' : ''}`}>
              <span
                className={`grid size-8 place-items-center rounded-full text-base ring-1 ${
                  s.state === 'done'
                    ? 'bg-emerald-400/10 text-emerald-300 ring-emerald-400/25'
                    : s.state === 'active'
                      ? 'bg-brand-500/15 text-brand-400 ring-brand-500/30'
                      : 'bg-white/[0.03] text-zinc-600 ring-white/[0.06]'
                }`}
              >
                {s.state === 'done' ? <CircleCheck className="icon" /> : s.state === 'active' && working ? <Spinner /> : <s.icon className="icon" />}
              </span>
              <span className={`text-[13px] font-medium ${s.state === 'todo' ? 'text-zinc-500' : 'text-zinc-100'}`}>{s.label}</span>
            </li>
          ))}
        </ol>

        <div className="mt-4 border-t border-white/[0.06] bg-ink-850/60 px-7 py-5">
          <div className="flex items-start gap-2.5">
            {setup.phase === 'error' && <TriangleAlert className="icon mt-0.5 text-base text-amber-300" />}
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-medium text-zinc-100">{setup.message}</p>
              {setup.detail && <p className="mt-1 text-xs break-words text-zinc-400">{setup.detail}</p>}
            </div>
          </div>
          {setup.phase === 'downloading' && (
            <div className="mt-4">
              <div className="h-1.5 overflow-hidden rounded-full bg-brand-500/15">
                <div className="h-full rounded-full bg-brand-500 transition-[width] duration-300" style={{ width: `${Math.round((setup.progress || 0) * 100)}%` }} />
              </div>
              <p className="mt-1.5 text-right text-xs tabular-nums text-zinc-500">{Math.round((setup.progress || 0) * 100)}%</p>
            </div>
          )}
          {setup.phase === 'installing' && (
            <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-brand-500/15">
              <div className="h-full w-1/3 animate-[indeterminate_1.2s_ease-in-out_infinite] rounded-full bg-brand-500" />
            </div>
          )}
          <div className="mt-5 flex items-center justify-between gap-3">
            <button className="btn btn-ghost btn-sm" onClick={onDismiss}>
              Browse servers first
            </button>
            {setup.action && !working && (
              <button className="btn btn-primary" disabled={!!busy} onClick={run('fix', () => api.fixSetup())}>
                {busy ? <Spinner /> : null} {setup.action} <ArrowRight className="icon" />
              </button>
            )}
          </div>
        </div>
      </div>
      <style>{'@keyframes indeterminate{0%{transform:translateX(-100%)}100%{transform:translateX(300%)}}'}</style>
    </div>
  )
}

function stepsFor(setup, info) {
  const installed = !!setup.openvpn
  const p = setup.phase
  const install = installed ? 'done' : 'active'
  const access = !installed ? 'todo' : p === 'needs-permission' || p === 'error' ? 'active' : 'done'
  return [
    { label: p === 'downloading' ? 'Downloading OpenVPN' : 'Install OpenVPN', icon: Download, state: install },
    { label: info?.os === 'windows' ? 'Set up the network adapter' : 'Allow the app to create the tunnel', icon: KeyRound, state: access },
    { label: 'Ready to connect', icon: ShieldCheck, state: 'todo' },
  ]
}
