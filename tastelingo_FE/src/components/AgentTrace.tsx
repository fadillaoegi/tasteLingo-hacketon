import { useState } from 'react'
import { ChevronDown, Wrench } from 'lucide-react'
import type { AgentRun, TraceStep } from '../api'

const kindLabels: Record<TraceStep['kind'], string> = {
  thought: 'Reasoning',
  tool_call: 'Tool call',
  tool_result: 'Result',
  final: 'Final',
  error: 'Error',
}

export function AgentTrace({ run }: { run: AgentRun }) {
  const [open, setOpen] = useState(false)
  const calls = run.steps.filter((s) => s.kind === 'tool_call').length
  return (
    <section className="card">
      <button className="flex w-full items-center justify-between gap-2 text-left" onClick={() => setOpen(!open)} aria-expanded={open}>
        <span className="flex items-center gap-2 font-semibold">
          <Wrench className="size-4 text-teal-700" /> Agent trace
          <span className="text-sm font-normal text-stone-500">{calls} tool calls · {run.status}</span>
        </span>
        <ChevronDown className={open ? 'size-5 rotate-180 transition' : 'size-5 transition'} />
      </button>
      {run.error && <p className="mt-2 text-sm text-red-600">{run.error}</p>}
      {open && (
        <ol className="mt-4 space-y-2">
          {run.steps.map((s) => (
            <li key={s.index} className={`rounded-lg border px-3 py-2 text-sm ${s.kind === 'error' ? 'border-red-200 bg-red-50' : 'border-stone-200'}`}>
              <p className="flex flex-wrap items-center gap-2 text-xs text-stone-500">
                <span className="font-semibold text-stone-700">{kindLabels[s.kind]}</span>
                {s.tool && <code className="rounded bg-stone-100 px-1.5 py-0.5 text-teal-800">{s.tool}</code>}
                {s.duration_ms !== undefined && <span>{s.duration_ms} ms</span>}
                {s.model && <span className="rounded bg-amber-50 px-1.5 py-0.5 text-amber-900" title="Model that produced this step">{s.model}</span>}
              </p>
              {s.input !== undefined && <pre className="trace-pre">{JSON.stringify(s.input, null, 2)}</pre>}
              {s.output && <pre className="trace-pre">{pretty(s.output)}</pre>}
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}

function pretty(text: string) {
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}
