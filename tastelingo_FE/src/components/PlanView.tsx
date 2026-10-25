import { useState } from 'react'
import { CalendarDays, Clock, Dumbbell, Layers3, Loader2, RefreshCw, Wand2 } from 'lucide-react'
import { feedbackLabels, typeLabels, type AgentRun, type Feedback, type FeedbackKind, type Learner, type Plan } from '../api'
import { TypeIcon } from './TypeIcon'
import { ExercisePanel } from './ExercisePanel'

interface Props {
  learner: Learner
  plan: Plan
  feedback: Feedback[]
  busy: boolean
  onRebuild: () => void
  onFeedback: (day: number, kind: FeedbackKind) => void
  onAdapt: () => void
  onRun: (run: AgentRun) => void
}

const kinds: FeedbackKind[] = ['like', 'dislike', 'too_hard', 'too_easy']

export function PlanView({ learner, plan, feedback, busy, onRebuild, onFeedback, onAdapt, onRun }: Props) {
  const [practice, setPractice] = useState<number | null>(null)
  return (
    <div className="space-y-6 plan-dashboard">
      <section className="card plan-overview">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="eyebrow">YOUR INTERESTS, A NEW CHAPTER</p>
            <h2 className="plan-overview-title">{learner.name}'s <span>learning week.</span></h2>
            <p className="plan-overview-subtitle">{learner.level} English · {learner.goal} · plan v{plan.version}</p>
          </div>
          <button className="btn-ghost" onClick={onRebuild} disabled={busy}>
            <RefreshCw className={busy ? 'size-4 animate-spin' : 'size-4'} /> Rebuild
          </button>
        </div>
        <div className="plan-facts">
          <span><CalendarDays size={15} /><strong>{plan.days.length} days</strong> of discovery</span>
          <span><Clock size={15} /><strong>{learner.daily_minutes} min</strong> a day</span>
          <span><Layers3 size={15} /><strong>{new Set(plan.days.map(day => day.entity.type)).size} formats</strong> to explore</span>
        </div>
        <p className="mt-3 text-stone-700">{plan.summary}</p>
        <div className="mt-3 flex flex-wrap gap-2">
          {plan.themes.map((t) => <span key={t} className="chip">{t}</span>)}
        </div>
        <p className="mt-4 text-sm text-stone-500">Built from: {learner.favorites.map((f) => f.name).join(', ')}</p>
        {plan.changes && (
          <p className="mt-3 rounded-lg border border-teal-200 bg-teal-50 p-3 text-sm text-teal-900"><span className="font-semibold">What changed:</span> {plan.changes}</p>
        )}
      </section>

      {feedback.length > 0 && (
        <section className="card flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-stone-700">
            {feedback.length} feedback note{feedback.length > 1 ? 's' : ''} on this plan. The agent can replan with them.
          </p>
          <button className="btn-primary py-2" onClick={onAdapt} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Wand2 className="size-4" />} Adapt my plan
          </button>
        </section>
      )}

      <ol className="day-grid">
        {plan.days.map((d, dayIndex) => {
          const given = feedback.filter((f) => f.day === d.day).map((f) => f.kind)
          return (
            <li key={d.day} className={`card day-card ${practice === d.day ? 'is-practising' : ''}`} style={{ animationDelay: `${dayIndex * 45}ms` }}>
              <div className="flex gap-4">
                <div className="day-number">
                  <span className="text-[10px] font-semibold uppercase">Day</span>
                  <span className="text-lg leading-none font-bold">{d.day}</span>
                </div>
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2 day-card-title">
                    <TypeIcon type={d.entity.type} small />
                    <h3 className="font-semibold">{d.entity.name}</h3>
                    {d.entity.source === 'mock' && <span className="badge-warn">mock data</span>}
                  </div>
                  <p className="text-xs text-stone-500">
                    {typeLabels[d.entity.type]}{d.entity.year ? ` · ${d.entity.year}` : ''} · <span className="capitalize">{d.focus}</span> ·{' '}
                    <Clock className="inline size-3" /> {d.minutes} min
                    {d.entity.affinity ? ` · affinity ${d.entity.affinity.toFixed(2)}` : ''}
                  </p>
                  <p>{d.activity}</p>
                  <p className="text-sm text-stone-600"><span className="font-medium text-stone-800">Why for you:</span> {d.why}</p>
                  {d.entity.tags && d.entity.tags.length > 0 && (
                    <div className="flex flex-wrap gap-1 pt-1">
                      {d.entity.tags.slice(0, 5).map((t) => <span key={t} className="tag">{t}</span>)}
                    </div>
                  )}
                  <div className="flex flex-wrap items-center gap-1.5 pt-2">
                    <button className="btn-ghost" onClick={() => setPractice(practice === d.day ? null : d.day)} aria-expanded={practice === d.day}>
                      <Dumbbell className="size-4" /> {practice === d.day ? 'Hide practice' : 'Practice'}
                    </button>
                    {kinds.map((k) => (
                      <button key={k} className={given.includes(k) ? 'toggle toggle-on py-1 text-xs' : 'toggle py-1 text-xs'} disabled={busy || given.includes(k)} onClick={() => onFeedback(d.day, k)}>
                        {feedbackLabels[k]}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
              {practice === d.day && <ExercisePanel planId={plan.id} day={d.day} onRun={onRun} />}
            </li>
          )
        })}
      </ol>
    </div>
  )
}
