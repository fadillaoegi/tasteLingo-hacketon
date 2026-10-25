// @vitest-environment jsdom
import { StrictMode, act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { ApiError, type AgentRun, type Learner, type LearnerSession, type Plan } from './api'

const mocks = vi.hoisted(() => ({
  health: vi.fn(), session: vi.fn(), demo: vi.fn(), buildPlan: vi.fn(), latestPlan: vi.fn(),
}))
vi.mock('./api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api')>()
  return { ...actual, api: { ...actual.api, ...mocks } }
})

const learner: Learner = { id: 'demo-learner', name: 'Rina', level: 'B1', goal: 'work', daily_minutes: 30, favorites: [] }
const plan: Plan = { id: 'p1', learner_id: learner.id, run_id: 'r1', version: 1, grounding: 'taste_graph', themes: [], summary: 'A saved learning plan', days: [] }
const run: AgentRun = { id: 'r1', status: 'succeeded', taste_mode: 'qloo', llm_mode: 'gemini', steps: [] }
const session = (withPlan = false): LearnerSession => ({ learner, plan: withPlan ? plan : null, run: withPlan ? run : null, feedback: [], modes: { taste_graph: 'qloo', llm: 'gemini' } })

let root: Root
let container: HTMLDivElement

beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  localStorage.clear()
  vi.resetAllMocks()
  mocks.health.mockResolvedValue({ modes: { taste_graph: 'qloo', llm: 'gemini' } })
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove() })

async function mount() { await act(async () => root.render(<StrictMode><App /></StrictMode>)) }
function button(text: string): HTMLButtonElement {
  const found = Array.from(container.querySelectorAll('button')).find(b => b.textContent?.includes(text))
  if (!found) throw new Error(`Missing button ${text}: ${container.textContent}`)
  return found
}
async function click(text: string) { await act(async () => button(text).click()) }
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('demo and saved-profile lifecycle', () => {
  it('does not fetch a missing plan while a new demo plan is being created', async () => {
    const pending = deferred<{ plan: Plan; run: AgentRun }>()
    mocks.demo.mockResolvedValue(learner)
    mocks.buildPlan.mockReturnValue(pending.promise)
    await mount()
    await click('Try the demo')
    expect(mocks.buildPlan).toHaveBeenCalledTimes(1)
    expect(mocks.session).not.toHaveBeenCalled()
    expect(mocks.latestPlan).not.toHaveBeenCalled()
    expect(container.textContent).toContain('Building your 7-day plan')
    expect(container.textContent).toContain('elapsed')
    await act(async () => pending.resolve({ plan, run }))
    expect(container.textContent).toContain(plan.summary)
    expect(container.textContent).not.toContain('Building your 7-day plan')
  })

  it('restores a saved learner with no plan as a retryable normal state', async () => {
    localStorage.setItem('tastelingo.learner', JSON.stringify(learner))
    mocks.session.mockResolvedValue(session())
    mocks.buildPlan.mockResolvedValue({ plan, run })
    await mount()
    expect(container.textContent).toContain('Your profile is ready, Rina')
    expect(mocks.latestPlan).not.toHaveBeenCalled()
    expect(mocks.buildPlan).not.toHaveBeenCalled()
    await click('Build my plan')
    expect(mocks.buildPlan).toHaveBeenCalledWith(learner.id, expect.any(AbortSignal))
    expect(container.textContent).toContain(plan.summary)
  })

  it('clears a stale browser profile when the server no longer has it', async () => {
    localStorage.setItem('tastelingo.learner', JSON.stringify(learner))
    mocks.session.mockResolvedValue({ ...session(), learner: null })
    await mount()
    expect(localStorage.getItem('tastelingo.learner')).toBeNull()
    expect(container.textContent).toContain('Try the demo')
    expect(mocks.latestPlan).not.toHaveBeenCalled()
  })

  it('releases loading after a failed plan and retries with the same saved profile', async () => {
    mocks.demo.mockResolvedValue(learner)
    mocks.buildPlan.mockRejectedValueOnce(new ApiError('the agent could not finish', 502, { run: { ...run, status: 'failed' } }))
    mocks.buildPlan.mockResolvedValueOnce({ plan, run })
    await mount()
    await click('Try the demo')
    expect(container.textContent).toContain('Your profile is saved')
    expect(container.textContent).toContain('Your profile is ready, Rina')
    await click('Build my plan')
    expect(mocks.demo).toHaveBeenCalledTimes(1)
    expect(mocks.buildPlan).toHaveBeenCalledTimes(2)
    expect(container.textContent).toContain(plan.summary)
  })

  it('cancels loading and ignores a late completion after starting over', async () => {
    const pending = deferred<{ plan: Plan; run: AgentRun }>()
    mocks.demo.mockResolvedValue(learner)
    mocks.buildPlan.mockReturnValue(pending.promise)
    await mount()
    await click('Try the demo')
    const signal = mocks.buildPlan.mock.calls[0][1] as AbortSignal
    await click('Cancel')
    expect(signal.aborted).toBe(true)
    expect(container.textContent).toContain('Your profile is ready')
    await act(async () => container.querySelector<HTMLButtonElement>('.brand-home')!.click())
    await act(async () => pending.resolve({ plan, run }))
    expect(container.textContent).toContain('Try the demo')
    expect(container.textContent).not.toContain(plan.summary)
    expect(localStorage.getItem('tastelingo.learner')).toBeNull()
  })

  it('opens the previous successful demo without generating another plan', async () => {
    localStorage.setItem('tastelingo.demo', learner.id)
    mocks.session.mockResolvedValue(session(true))
    await mount()
    await click('Try the demo')
    expect(mocks.demo).not.toHaveBeenCalled()
    expect(mocks.buildPlan).not.toHaveBeenCalled()
    expect(container.textContent).toContain(plan.summary)
  })
})
