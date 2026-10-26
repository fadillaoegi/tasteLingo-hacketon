export type EntityType = 'movie' | 'tv_show' | 'book' | 'podcast' | 'artist'

export interface Entity {
  id: string
  name: string
  type: EntityType
  description?: string
  year?: number
  image_url?: string
  tags?: string[]
  affinity?: number
  source: 'qloo' | 'mock' | 'user' | 'llm'
}

export interface Learner {
  id: string
  name: string
  level: string
  goal: string
  daily_minutes: number
  favorites: Entity[]
}

export interface PlanDay {
  day: number
  entity: Entity
  activity: string
  focus: string
  minutes: number
  why: string
}

export interface Plan {
  id: string
  learner_id: string
  run_id: string
  version: number
  grounding: string
  themes: string[]
  summary: string
  days: PlanDay[]
  changes?: string
}

export interface TraceStep {
  index: number
  kind: 'thought' | 'tool_call' | 'tool_result' | 'final' | 'error'
  tool?: string
  input?: unknown
  output?: string
  duration_ms?: number
  model?: string
}

export interface AgentRun {
  id: string
  status: string
  taste_mode: string
  llm_mode: string
  steps: TraceStep[]
  error?: string
}

export interface Modes {
  taste_graph: string
  llm: string
}

export interface LearnerSession {
  learner: Learner | null
  plan: Plan | null
  run: AgentRun | null
  feedback: Feedback[]
  modes: Modes
}

export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly body: unknown) {
    super(message)
  }
}

// Same-origin API under the app's base path, so the app also works from a subpath such as /tastelingo/.
const apiPrefix = `${import.meta.env.BASE_URL.replace(/\/$/, '')}/api`

async function request<T>(path: string, init?: RequestInit, timeoutMs = 20_000): Promise<T> {
  const controller = new AbortController()
  const abort = () => controller.abort()
  if (init?.signal?.aborted) controller.abort()
  init?.signal?.addEventListener('abort', abort, { once: true })
  let timedOut = false
  const timer = setTimeout(() => { timedOut = true; controller.abort() }, timeoutMs)
  try {
    const res = await fetch(`${apiPrefix}${path}`, { ...init, signal: controller.signal, headers: { 'Content-Type': 'application/json', ...init?.headers } })
    const body = await res.json().catch((err: unknown) => {
      if (controller.signal.aborted) throw err
      return null
    })
    if (!res.ok) throw new ApiError(body?.error ?? `Request failed (${res.status})`, res.status, body)
    return body as T
  } catch (err) {
    if (timedOut && !init?.signal?.aborted) throw new Error('This request took too long. Please try again; your profile is saved.')
    throw err
  } finally {
    clearTimeout(timer)
    init?.signal?.removeEventListener('abort', abort)
  }
}

export const api = {
  health: () => request<{ modes: Modes }>('/health'),
  search: (q: string, type: EntityType | '') =>
    request<{ entities: Entity[]; source: string }>(`/search?${new URLSearchParams({ q, type })}`),
  createLearner: (l: Omit<Learner, 'id'>, signal?: AbortSignal) => request<Learner>('/learners', { method: 'POST', body: JSON.stringify(l), signal }),
  buildPlan: (learnerId: string, signal?: AbortSignal) => request<{ plan: Plan; run: AgentRun }>(`/learners/${learnerId}/plan`, { method: 'POST', signal }, 130_000),
  session: (learnerId: string, signal?: AbortSignal) => request<LearnerSession>(`/learners/${learnerId}/session`, { signal }),
  latestPlan: (learnerId: string) => request<Plan>(`/learners/${learnerId}/plan`),
  run: (id: string) => request<AgentRun>(`/runs/${id}`),
  demo: (signal?: AbortSignal) => request<Learner>('/demo', { method: 'POST', signal }, 50_000),
  buildComparison: (learnerId: string) => request<{ plan: Plan; run: AgentRun }>(`/learners/${learnerId}/compare`, { method: 'POST' }, 130_000),
  latestComparison: (learnerId: string) => request<Plan>(`/learners/${learnerId}/compare`),
  feedback: (learnerId: string, planId: string, day: number, kind: FeedbackKind) =>
    request<Feedback>(`/learners/${learnerId}/feedback`, { method: 'POST', body: JSON.stringify({ plan_id: planId, day, kind }) }),
  planFeedback: (planId: string) => request<{ feedback: Feedback[] }>(`/plans/${planId}/feedback`),
  adapt: (learnerId: string, signal?: AbortSignal) => request<{ plan: Plan; run: AgentRun }>(`/learners/${learnerId}/adapt`, { method: 'POST', signal }, 130_000),
  exercise: (planId: string, day: number) => request<{ exercise: Exercise; run: AgentRun | null }>(`/plans/${planId}/days/${day}/exercise`, { method: 'POST' }, 130_000),
  attempt: (exerciseId: string, answers: number[], response: string) =>
    request<{ attempt: Attempt; exercise: Exercise }>(`/exercises/${exerciseId}/attempts`, { method: 'POST', body: JSON.stringify({ answers, response }) }, 130_000),
}

export const typeLabels: Record<EntityType, string> = {
  movie: 'Film',
  tv_show: 'TV show',
  book: 'Book',
  podcast: 'Podcast',
  artist: 'Music',
}

export type FeedbackKind = 'too_hard' | 'too_easy' | 'dislike' | 'like'

export interface Feedback {
  id: string
  plan_id: string
  day: number
  entity_id: string
  entity_name: string
  kind: FeedbackKind
}

export interface Exercise {
  id: string
  plan_id: string
  day: number
  run_id: string
  vocabulary: { term: string; meaning: string; example: string }[]
  questions: { question: string; options: string[]; answer?: number }[]
  prompt: { kind: 'speaking' | 'writing'; text: string }
}

export interface Attempt {
  id: string
  answers: number[]
  correct: boolean[]
  score: number
  response: string
  feedback: string
}

export const feedbackLabels: Record<FeedbackKind, string> = {
  like: 'Love it',
  dislike: 'Not my taste',
  too_hard: 'Too hard',
  too_easy: 'Too easy',
}
