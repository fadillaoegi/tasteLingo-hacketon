import { useEffect, useState } from 'react'
import { Check, Loader2, Send, X } from 'lucide-react'
import { api, type AgentRun, type Attempt, type Exercise } from '../api'

interface Props {
  planId: string
  day: number
  onRun: (run: AgentRun) => void
}

export function ExercisePanel({ planId, day, onRun }: Props) {
  const [exercise, setExercise] = useState<Exercise | null>(null)
  const [answers, setAnswers] = useState<number[]>([])
  const [response, setResponse] = useState('')
  const [result, setResult] = useState<{ attempt: Attempt; exercise: Exercise } | null>(null)
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let live = true
    api
      .exercise(planId, day)
      .then((res) => {
        if (!live) return
        setExercise(res.exercise)
        setAnswers(res.exercise.questions.map(() => -1))
        if (res.run) onRun(res.run)
      })
      .catch((err) => live && setError(err.message))
      .finally(() => live && setBusy(false))
    return () => {
      live = false
    }
  }, [planId, day, onRun])

  async function submit() {
    if (!exercise) return
    setBusy(true)
    setError('')
    try {
      setResult(await api.attempt(exercise.id, answers, response))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not submit')
    } finally {
      setBusy(false)
    }
  }

  if (!exercise) {
    return (
      <div className="mt-3 rounded-lg bg-stone-50 p-3 text-sm text-stone-600">
        {error ? <span className="text-red-600">{error}</span> : <span className="flex items-center gap-2"><Loader2 className="size-4 animate-spin" /> The agent is writing your exercise…</span>}
      </div>
    )
  }

  const scored = result?.exercise.questions
  return (
    <div className="mt-3 space-y-4 rounded-lg bg-stone-50 p-3 sm:p-4">
      <div>
        <h4 className="mb-2 text-sm font-semibold">Vocabulary</h4>
        <ul className="grid gap-2 sm:grid-cols-2">
          {exercise.vocabulary.map((v) => (
            <li key={v.term} className="rounded-lg border border-stone-200 bg-white p-2.5 text-sm">
              <p className="font-semibold text-teal-800">{v.term}</p>
              <p className="text-stone-600">{v.meaning}</p>
              <p className="mt-1 text-stone-500 italic">“{v.example}”</p>
            </li>
          ))}
        </ul>
      </div>

      <div className="space-y-3">
        <h4 className="text-sm font-semibold">Check your understanding</h4>
        {exercise.questions.map((q, qi) => (
          <fieldset key={qi} className="text-sm">
            <legend className="mb-1.5 font-medium">{qi + 1}. {q.question}</legend>
            <div className="grid gap-1.5">
              {q.options.map((o, oi) => {
                const correct = scored?.[qi].answer === oi
                const chosen = answers[qi] === oi
                const tone = scored ? (correct ? 'border-emerald-400 bg-emerald-50' : chosen ? 'border-red-300 bg-red-50' : 'border-stone-200 bg-white') : chosen ? 'border-teal-600 bg-teal-50' : 'border-stone-200 bg-white'
                return (
                  <label key={oi} className={`flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 ${tone}`}>
                    <input type="radio" name={`q-${exercise.id}-${qi}`} checked={chosen} disabled={!!result} onChange={() => setAnswers(answers.map((a, i) => (i === qi ? oi : a)))} />
                    <span className="flex-1">{o}</span>
                    {scored && correct && <Check className="size-4 text-emerald-700" />}
                    {scored && chosen && !correct && <X className="size-4 text-red-600" />}
                  </label>
                )
              })}
            </div>
          </fieldset>
        ))}
      </div>

      <label className="block text-sm">
        <span className="mb-1.5 block font-semibold capitalize">{exercise.prompt.kind} task</span>
        <span className="mb-2 block text-stone-700">{exercise.prompt.text}</span>
        <textarea className="input min-h-24 w-full" maxLength={1500} value={response} disabled={!!result} onChange={(e) => setResponse(e.target.value)} placeholder={exercise.prompt.kind === 'speaking' ? 'Say it out loud first, then type what you said…' : 'Write your answer…'} />
      </label>

      {error && <p className="text-sm text-red-600">{error}</p>}
      {result ? (
        <div className="rounded-lg border border-teal-200 bg-white p-3 text-sm">
          <p className="font-semibold">Score: {result.attempt.score} / {result.attempt.correct.length}</p>
          {result.attempt.feedback && <p className="mt-1 text-stone-700">{result.attempt.feedback}</p>}
        </div>
      ) : (
        <button className="btn-secondary" onClick={submit} disabled={busy || answers.some((a) => a < 0)}>
          {busy ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />} Check answers
        </button>
      )}
    </div>
  )
}
