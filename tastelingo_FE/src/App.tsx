import { useCallback, useEffect, useRef, useState } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { api, ApiError, type AgentRun, type Feedback, type FeedbackKind, type Learner, type LearnerSession, type Modes, type Plan } from './api'
import { Onboarding } from './components/Onboarding'
import { PlanView } from './components/PlanView'
import { CompareView } from './components/CompareView'
import { AgentTrace } from './components/AgentTrace'
import { BrandLogo } from './components/BrandLogo'
import { PlanProgress, type LoadingPhase } from './components/PlanProgress'

const storageKey = 'tastelingo.learner'
const demoKey = 'tastelingo.demo'

function readStored(): Learner | null {
  try {
    const raw = localStorage.getItem(storageKey)
    const value = raw ? JSON.parse(raw) as Learner : null
    return value && typeof value.id === 'string' && typeof value.name === 'string' && Array.isArray(value.favorites) ? value : null
  } catch {
    return null
  }
}

function store(l: Learner | null) {
  try {
    if (l) localStorage.setItem(storageKey, JSON.stringify(l))
    else localStorage.removeItem(storageKey)
  } catch {
    // Storage may be blocked; the session still works without it.
  }
}

type Tab = 'plan' | 'compare'

export default function App() {
  const [modes, setModes] = useState<Modes | null>(null)
  const [learner, setLearner] = useState<Learner | null>(readStored)
  const [plan, setPlan] = useState<Plan | null>(null)
  const [feedback, setFeedback] = useState<Feedback[]>([])
  const [run, setRun] = useState<AgentRun | null>(null)
  const [tab, setTab] = useState<Tab>('plan')
  const [phase, setPhase] = useState<LoadingPhase | null>(null)
  const [error, setError] = useState('')
  const activeRequest = useRef<AbortController | null>(null)
  const busy = phase !== null
  const showRun = useCallback((r: AgentRun) => setRun(r), [])

  useEffect(() => {
    api.health().then((h) => setModes(h.modes)).catch(() => setError('The API is not reachable.'))
  }, [])

  // Restore once on mount. A newly created learner must not start a competing
  // GET while its POST /plan is still running. StrictMode cleanup cancels stale work.
  useEffect(() => {
    const saved = readStored()
    if (!saved) return
    const controller = new AbortController()
    activeRequest.current = controller
    setPhase('restoring')
    api.session(saved.id, controller.signal)
      .then((session) => {
        if (activeRequest.current !== controller || controller.signal.aborted) return
        restore(session)
      })
      .catch((err: unknown) => {
        if (activeRequest.current === controller && !controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not open your saved profile.')
      })
      .finally(() => finish(controller))
    return () => controller.abort()
  }, [])

  useEffect(() => () => activeRequest.current?.abort(), [])

  function finish(controller: AbortController) {
    if (activeRequest.current !== controller) return
    activeRequest.current = null
    setPhase(null)
  }

  function restore(session: LearnerSession) {
    setLearner(session.learner)
    store(session.learner)
    setPlan(session.plan)
    setFeedback(session.feedback)
    setRun(session.run)
    setModes(session.modes)
    setTab('plan')
  }

  function applyPlan(res: { plan: Plan; run: AgentRun }) {
    setPlan(res.plan)
    setFeedback([])
    setRun(res.run)
    setTab('plan')
  }

  function failed(err: unknown) {
    if (err instanceof ApiError && err.body && typeof err.body === 'object' && 'run' in err.body) setRun((err.body as { run: AgentRun }).run)
    setError(err instanceof ApiError && err.status === 502
      ? 'We could not finish your plan. Your profile is saved — please try again.'
      : err instanceof Error ? err.message : 'Could not build your plan. Please try again.')
  }

  // runAgent wraps a call that returns a plan and its trace; a failed run still shows its trace.
  async function runAgent(call: (signal: AbortSignal) => Promise<{ plan: Plan; run: AgentRun }>) {
    if (activeRequest.current) return
    const controller = new AbortController()
    activeRequest.current = controller
    setPhase('building')
    setError('')
    try {
      const res = await call(controller.signal)
      if (activeRequest.current === controller && !controller.signal.aborted) applyPlan(res)
    } catch (err) {
      if (activeRequest.current === controller && !controller.signal.aborted) failed(err)
    } finally {
      finish(controller)
    }
  }

  async function start(create: (signal: AbortSignal) => Promise<Learner>, demo = false) {
    if (activeRequest.current) return
    const controller = new AbortController()
    activeRequest.current = controller
    setPhase('creating')
    setError('')
    setRun(null)
    try {
      let l: Learner | null = null
      let cached: string | null = null
      try { if (demo) cached = localStorage.getItem(demoKey) } catch { /* Storage is optional. */ }
      if (cached) {
        const session = await api.session(cached, controller.signal)
        if (activeRequest.current !== controller || controller.signal.aborted) return
        if (session.learner && session.plan && session.run?.taste_mode === session.modes.taste_graph && session.run?.llm_mode === session.modes.llm) {
          restore(session)
          return
        }
        if (session.learner && !session.plan) l = session.learner
      }
      l ??= await create(controller.signal)
      if (activeRequest.current !== controller || controller.signal.aborted) return
      setLearner(l)
      store(l)
      if (demo) { try { localStorage.setItem(demoKey, l.id) } catch { /* Storage is optional. */ } }
      setPhase('building')
      const res = await api.buildPlan(l.id, controller.signal)
      if (activeRequest.current === controller && !controller.signal.aborted) applyPlan(res)
    } catch (err) {
      if (activeRequest.current === controller && !controller.signal.aborted) failed(err)
    } finally {
      finish(controller)
    }
  }

  async function giveFeedback(day: number, kind: FeedbackKind) {
    if (!learner || !plan) return
    try {
      const f = await api.feedback(learner.id, plan.id, day, kind)
      setFeedback((list) => [...list, f])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save feedback')
    }
  }

  function reset() {
    activeRequest.current?.abort()
    activeRequest.current = null
    setPhase(null)
    store(null)
    setLearner(null)
    setPlan(null)
    setFeedback([])
    setRun(null)
    setTab('plan')
    setError('')
  }

  function cancel() {
    activeRequest.current?.abort()
    activeRequest.current = null
    setPhase(null)
    setError('')
  }

  const mock = modes && (modes.taste_graph !== 'qloo' || modes.llm === 'mock')

  return (
    <div className="app-shell">
      <header className="site-header">
        <div className="site-header-inner">
          <button onClick={reset} className="brand-home text-left" title="Start over" aria-label="TasteLingo — start over">
            <BrandLogo />
          </button>
          <span className="header-note">A world of interests.<br /><strong>A little English, every day.</strong></span>
          {modes && (
            <div className="provider-status">
              <span className={modes.taste_graph === 'qloo' ? 'badge-ok' : 'badge-warn'}>Taste graph: {modes.taste_graph === 'qloo' ? 'Qloo' : 'mock'}</span>
              <span className={modes.llm === 'mock' ? 'badge-warn' : 'badge-ok'}>LLM: {modes.llm}</span>
            </div>
          )}
        </div>
      </header>
      {(learner || busy) && <h1 className="sr-only">TasteLingo — learn English through what you love</h1>}
      <main className={`app-main ${learner || busy ? 'learning-workspace' : 'onboarding-workspace'}`}>
        {mock && (
          <p className="flex items-start gap-2 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
            <AlertTriangle className="mt-0.5 size-4 shrink-0" />
            Development mode: some data is synthetic and labelled “mock”. It is not real Qloo data.
          </p>
        )}
        {error && <p className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p>}
        {phase && <PlanProgress phase={phase} onCancel={cancel} />}
        {learner && plan ? (
          <>
            <nav className="workspace-tabs" aria-label="Views">
              {(['plan', 'compare'] as Tab[]).map((t) => (
                <button key={t} onClick={() => setTab(t)} aria-current={tab === t}>
                  {t === 'plan' ? 'My plan' : 'Compare: without Qloo'}
                </button>
              ))}
            </nav>
            {tab === 'plan' ? (
              <PlanView
                learner={learner}
                plan={plan}
                feedback={feedback}
                busy={busy}
                onRebuild={() => runAgent((signal) => api.buildPlan(learner.id, signal))}
                onFeedback={giveFeedback}
                onAdapt={() => runAgent((signal) => api.adapt(learner.id, signal))}
                onRun={showRun}
              />
            ) : (
              <CompareView learnerId={learner.id} plan={plan} tasteMode={modes?.taste_graph ?? 'mock'} onRun={showRun} />
            )}
          </>
        ) : learner && !busy ? (
          <section className="card space-y-3">
            <h2 className="section-title">Your profile is ready, {learner.name}</h2>
            <p className="muted">Your 7-day plan has not been completed yet. You can try again with your saved favorites.</p>
            <div className="flex flex-wrap gap-2">
              <button className="btn-primary" onClick={() => runAgent((signal) => api.buildPlan(learner.id, signal))}>
                <RefreshCw className="size-4" /> Build my plan
              </button>
              <button className="btn-ghost" onClick={reset}>Choose different favorites</button>
            </div>
          </section>
        ) : !busy ? (
          <Onboarding busy={busy} onSubmit={(input) => start((signal) => api.createLearner(input, signal))} onDemo={() => start(api.demo, true)} />
        ) : null}
        {run && <AgentTrace run={run} />}
      </main>
      <footer className="site-footer"><span>TasteLingo<span className="text-teal-700">.</span></span><p>A little culture. A little practice. A path that feels like you.</p><a href="#root">Back to top ↑</a></footer>
    </div>
  )
}
