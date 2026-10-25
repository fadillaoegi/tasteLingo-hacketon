import { useEffect, useState } from 'react'
import { BadgeCheck, CircleHelp, Loader2, Scale } from 'lucide-react'
import { api, ApiError, typeLabels, type AgentRun, type Plan } from '../api'
import { TypeIcon } from './TypeIcon'

interface Props {
  learnerId: string
  plan: Plan
  tasteMode: string
  onRun: (run: AgentRun) => void
}

// CompareView puts a plan from the LLM alone next to the taste-graph plan, using the same learner facts.
export function CompareView({ learnerId, plan, tasteMode, onRun }: Props) {
  const [baseline, setBaseline] = useState<Plan | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .latestComparison(learnerId)
      .then(setBaseline)
      .catch((err) => !(err instanceof ApiError && err.status === 404) && setError(err.message))
      .finally(() => setLoading(false))
  }, [learnerId])

  async function generate() {
    setBusy(true)
    setError('')
    try {
      const res = await api.buildComparison(learnerId)
      setBaseline(res.plan)
      onRun(res.run)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Comparison failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      <section className="card">
        <h2 className="flex items-center gap-2 font-semibold"><Scale className="size-5 text-teal-700" /> Without Qloo vs with TasteLingo</h2>
        <p className="muted mt-1">
          Both plans get the same learner facts (favorites, level, goal, time). The left one comes from the LLM alone; its titles are not checked against
          anything. The right one may only use titles returned by the {tasteMode === 'qloo' ? 'Qloo' : 'mock'} taste graph.
        </p>
        {!baseline && !loading && (
          <button className="btn-secondary mt-3" onClick={generate} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Scale className="size-4" />} Generate the LLM-only plan
          </button>
        )}
        {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
      </section>
      <div className="grid gap-4 md:grid-cols-2">
        <Column title="LLM only" subtitle="Generic suggestions, unverified" plan={baseline} placeholder={loading ? 'Loading…' : 'Not generated yet.'} />
        <Column title="TasteLingo" subtitle={`Grounded in the ${tasteMode === 'qloo' ? 'Qloo' : 'mock'} taste graph`} plan={plan} grounded />
      </div>
    </div>
  )
}

function Column({ title, subtitle, plan, grounded, placeholder }: { title: string; subtitle: string; plan: Plan | null; grounded?: boolean; placeholder?: string }) {
  return (
    <section className={`card ${grounded ? 'border-teal-300' : ''}`}>
      <h3 className="font-semibold">{title}</h3>
      <p className="mb-3 text-xs text-stone-500">{subtitle}</p>
      {!plan ? (
        <p className="text-sm text-stone-500">{placeholder}</p>
      ) : (
        <ol className="space-y-2">
          {plan.days.map((d) => (
            <li key={d.day} className="rounded-lg border border-stone-200 p-2.5 text-sm">
              <p className="flex items-center gap-2 font-medium">
                <span className="text-xs text-stone-500">D{d.day}</span>
                <TypeIcon type={d.entity.type} small />
                <span className="min-w-0 flex-1 truncate">{d.entity.name}</span>
                {d.entity.source === 'llm' ? (
                  <CircleHelp className="size-4 shrink-0 text-amber-600" aria-label="Not verified" />
                ) : (
                  <BadgeCheck className="size-4 shrink-0 text-teal-700" aria-label="From the taste graph" />
                )}
              </p>
              <p className="mt-1 text-xs text-stone-600">{typeLabels[d.entity.type]} · {d.why}</p>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
