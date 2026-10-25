import { useEffect, useState } from 'react'
import { BookOpen, Check, Loader2, Network, UserRound } from 'lucide-react'

export type LoadingPhase = 'restoring' | 'creating' | 'building'

const copy: Record<LoadingPhase, { title: string; description: string }> = {
  restoring: { title: 'Opening your saved profile', description: 'Checking whether your learning plan is ready.' },
  creating: { title: 'Getting your profile ready', description: 'Finding your favorites and preparing your learning profile.' },
  building: { title: 'Building your 7-day plan', description: 'Finding matching content and creating activities for your level.' },
}

export function PlanProgress({ phase, onCancel }: { phase: LoadingPhase; onCancel: () => void }) {
  const [seconds, setSeconds] = useState(0)
  useEffect(() => {
    const started = Date.now()
    setSeconds(0)
    const timer = setInterval(() => setSeconds(Math.floor((Date.now() - started) / 1000)), 1000)
    return () => clearInterval(timer)
  }, [phase])
  return (
    <section className="card plan-progress" aria-busy="true">
      <div className="progress-journey" aria-hidden="true" data-phase={phase}>
        <div className={`progress-stop ${phase === 'building' ? 'is-complete' : 'is-active'}`}>
          <span>{phase === 'building' ? <Check size={24} /> : <UserRound size={24} />}</span>
          <strong>Your profile</strong><small>{phase === 'building' ? 'Ready to go' : phase === 'restoring' ? 'Opening' : 'Getting ready'}</small>
        </div>
        <div className={`progress-connector ${phase === 'building' ? 'is-moving' : ''}`}><i /></div>
        <div className={`progress-stop ${phase === 'building' ? 'is-active' : ''}`}>
          <span><Network size={24} /></span><strong>Your connections</strong><small>{phase === 'building' ? 'Personalising' : 'Your interests guide us'}</small>
        </div>
        <div className={`progress-connector ${phase === 'building' ? 'is-moving' : ''}`}><i /></div>
        <div className="progress-stop">
          <span><BookOpen size={24} /></span><strong>Your week</strong><small>Opens when ready</small>
        </div>
      </div>
      <div className="flex items-start gap-3" role="status" aria-live="polite">
        <Loader2 className="mt-0.5 size-5 shrink-0 animate-spin text-teal-700 motion-reduce:animate-none" aria-hidden="true" />
        <div>
          <h2 className="section-title">{copy[phase].title}</h2>
          <p className="muted">{copy[phase].description}</p>
        </div>
      </div>
      <div className="mt-4 flex items-center justify-between border-t border-stone-100 pt-3">
        <span className="text-xs text-stone-500" aria-live="off">{seconds}s elapsed</span>
        <button className="btn-ghost" onClick={onCancel}>Cancel</button>
      </div>
      {phase === 'building' && seconds >= 20 && (
        <p className="mt-2 text-sm text-teal-800">This is taking a little longer. Your profile is saved, and you can cancel and try again.</p>
      )}
    </section>
  )
}
