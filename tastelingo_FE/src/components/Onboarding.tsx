import { useId, useState, type FormEvent } from 'react'
import { CheckCircle2, Loader2, Plus, Search, Sparkles, X } from 'lucide-react'
import { api, typeLabels, type Entity, type EntityType, type Learner } from '../api'
import { TypeIcon } from './TypeIcon'
import { TasteJourney } from './TasteJourney'

const levels = [
  {
    value: 'A2', label: 'Elementary',
    description: 'I understand short, simple sentences about everyday life.',
    example: 'For example: introducing myself or talking about my daily routine.',
  },
  {
    value: 'B1', label: 'Intermediate',
    description: 'I can follow everyday conversations and describe my experiences.',
    example: 'For example: telling someone about a trip or a film I enjoyed.',
  },
  {
    value: 'B2', label: 'Upper intermediate',
    description: 'I can discuss opinions and follow more complex articles or videos.',
    example: 'For example: explaining my point of view and giving reasons for it.',
  },
  {
    value: 'C1', label: 'Advanced',
    description: 'I understand complex ideas and express myself clearly and fluently.',
    example: 'For example: discussing detailed topics in work or study.',
  },
]
const goals = [
  { value: 'conversation', label: 'Conversation' },
  { value: 'work', label: 'Work & interviews' },
  { value: 'academic', label: 'Academic' },
]
const minutes = [15, 30, 60]

interface Props {
  busy: boolean
  onSubmit: (learner: Omit<Learner, 'id'>) => void
  onDemo: () => void
}

export function Onboarding({ busy, onSubmit, onDemo }: Props) {
  const levelHelpId = useId()
  const [name, setName] = useState('')
  const [query, setQuery] = useState('')
  const [type, setType] = useState<EntityType | ''>('')
  const [results, setResults] = useState<Entity[]>([])
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState('')
  const [favorites, setFavorites] = useState<Entity[]>([])
  const [level, setLevel] = useState('B1')
  const [goal, setGoal] = useState('work')
  const [daily, setDaily] = useState(30)

  async function search(e: FormEvent) {
    e.preventDefault()
    if (query.trim().length < 2) return
    setSearching(true)
    setSearchError('')
    try {
      setResults((await api.search(query.trim(), type)).entities)
    } catch (err) {
      setSearchError(err instanceof Error ? err.message : 'Search failed')
    } finally {
      setSearching(false)
    }
  }

  const add = (e: Entity) => {
    if (favorites.length >= 5 || favorites.some((f) => f.id === e.id)) return
    setFavorites([...favorites, e])
  }

  const canSubmit = name.trim() !== '' && favorites.length > 0 && !busy

  return (
    <div className="onboarding-layout">
      <TasteJourney favorites={favorites} level={level} daily={daily} onDemo={onDemo} busy={busy} />
      <div className="personalise-heading" id="personalise-form"><div><p className="eyebrow">LET'S MAKE THIS YOURS</p><h2>Your interests. Your starting point.</h2></div><span>Two small steps. A week of possibilities.</span></div>
      <div className="personalise-layout">
      <div className="personalise-fields">
      <section className="card onboarding-card">
        <h3 className="section-title"><span className="section-number">01</span> Tell us what you love</h3>
        <p className="muted mb-4">Add 1–5 films, shows, books, podcasts or artists you enjoy. They are the signals for your taste profile.</p>
        <form onSubmit={search} className="flex flex-col gap-2 sm:flex-row">
          <input className="input flex-1" placeholder="e.g. Spirited Away" value={query} onChange={(e) => setQuery(e.target.value)} aria-label="Search title or artist" />
          <select className="input sm:w-36" value={type} onChange={(e) => setType(e.target.value as EntityType | '')} aria-label="Type">
            <option value="">Any type</option>
            {(Object.keys(typeLabels) as EntityType[]).map((t) => (
              <option key={t} value={t}>{typeLabels[t]}</option>
            ))}
          </select>
          <button className="btn-secondary" disabled={searching || query.trim().length < 2}>
            {searching ? <Loader2 className="size-4 animate-spin" /> : <Search className="size-4" />} Search
          </button>
        </form>
        {searchError && <p className="mt-2 text-sm text-red-600">{searchError}</p>}
        {results.length > 0 && (
          <ul className="mt-3 divide-y divide-stone-200 rounded-lg border border-stone-200">
            {results.map((e) => (
              <li key={e.id} className="flex items-center gap-3 px-3 py-2">
                <TypeIcon type={e.type} />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">{e.name}</p>
                  <p className="text-xs text-stone-500">{typeLabels[e.type]}{e.year ? ` · ${e.year}` : ''}</p>
                </div>
                <button type="button" className="btn-ghost" onClick={() => add(e)} disabled={favorites.length >= 5 || favorites.some((f) => f.id === e.id)} aria-label={`Add ${e.name}`}>
                  <Plus className="size-4" /> Add
                </button>
              </li>
            ))}
          </ul>
        )}
        {favorites.length > 0 && (
          <div className="mt-4 flex flex-wrap gap-2">
            {favorites.map((f) => (
              <span key={f.id} className="chip">
                <TypeIcon type={f.type} small /> {f.name}
                <button type="button" onClick={() => setFavorites(favorites.filter((x) => x.id !== f.id))} aria-label={`Remove ${f.name}`}>
                  <X className="size-3.5" />
                </button>
              </span>
            ))}
          </div>
        )}
      </section>

      <section className="card onboarding-card space-y-4">
        <h3 className="section-title"><span className="section-number">02</span> About your English</h3>
        <label className="block">
          <span className="label">Your first name</span>
          <input className="input w-full" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} placeholder="Rina" />
        </label>
        <fieldset aria-describedby={levelHelpId}>
          <legend className="label">Your English level (CEFR)</legend>
          <p id={levelHelpId} className="muted mb-3">
            CEFR is a common scale for language ability. Choose the description closest to your English today.
            {' '}We use your choice to adjust the vocabulary, instructions and practice activities in your plan.
          </p>
          <div className="grid gap-3 sm:grid-cols-2">
            {levels.map((option) => (
              <label key={option.value} className={`relative cursor-pointer rounded-xl border p-3 transition-colors focus-within:ring-2 focus-within:ring-teal-600 focus-within:ring-offset-2 ${level === option.value ? 'border-teal-600 bg-teal-50' : 'border-stone-200 bg-white hover:border-teal-400'}`}>
                <input
                  type="radio"
                  name="english-level"
                  value={option.value}
                  checked={level === option.value}
                  onChange={() => setLevel(option.value)}
                  disabled={busy}
                  className="sr-only"
                  aria-label={`${option.value} — ${option.label}`}
                  aria-describedby={`${levelHelpId}-${option.value}`}
                />
                <span className="flex items-center justify-between gap-2">
                  <span className="text-sm font-semibold text-stone-900"><span className="text-teal-800">{option.value}</span> · {option.label}</span>
                  <CheckCircle2 className={`size-4 shrink-0 ${level === option.value ? 'text-teal-700' : 'text-stone-300'}`} aria-hidden="true" />
                </span>
                <span id={`${levelHelpId}-${option.value}`} className="mt-2 block text-sm leading-relaxed text-stone-700">
                  {option.description}
                  <span className="mt-1 block text-xs leading-relaxed text-stone-500">{option.example}</span>
                </span>
              </label>
            ))}
          </div>
          <p className="mt-3 text-xs leading-relaxed text-stone-500">
            Not sure? Choose A2 if you need short, simple English, or B1 if you can manage everyday conversations.
            {' '}This is your own estimate, not a formal test result.
          </p>
        </fieldset>
        <Choice label="Goal" options={goals} value={goal} onChange={setGoal} />
        <Choice label="Time per day" options={minutes.map((m) => ({ value: String(m), label: `${m} min` }))} value={String(daily)} onChange={(v) => setDaily(Number(v))} />
      </section>

      <button
        className="btn-primary w-full"
        disabled={!canSubmit}
        onClick={() => onSubmit({ name: name.trim(), level, goal, daily_minutes: daily, favorites: favorites.map(({ id, name, type }) => ({ id, name, type, source: 'user' })) })}
      >
        {busy ? <Loader2 className="size-5 animate-spin" /> : <Sparkles className="size-5" />}
        {busy ? 'The agent is building your plan…' : 'Build my 7-day plan'}
      </button>
      </div>
      <aside className="profile-preview" aria-label="Your plan preferences">
        <span className="preview-label">A PLAN THAT STARTS WITH YOU</span>
        <div className="profile-orb" aria-hidden="true"><img src={`${import.meta.env.BASE_URL}brand/tastelingo-mark-3d.webp`} alt="" width="128" height="128" /></div>
        <h3>A little more you.<br />A little more English.</h3>
        <p>Your choices shape the content and activities in your plan.</p>
        <dl>
          <div><dt>Your interests</dt><dd>{favorites.length ? `${favorites.length} favourite${favorites.length === 1 ? '' : 's'} selected` : 'Waiting for your favourites'}</dd></div>
          <div><dt>Your level</dt><dd>{level} · {levels.find(option => option.value === level)?.label}</dd></div>
          <div><dt>Your goal</dt><dd>{goals.find(option => option.value === goal)?.label}</dd></div>
          <div><dt>Your pace</dt><dd>{daily} minutes a day</dd></div>
        </dl>
        <span className="profile-preview-note"><Sparkles size={14} /> Your plan is created when you press Build.</span>
      </aside>
      </div>
    </div>
  )
}

function Choice({ label, options, value, onChange }: { label: string; options: { value: string; label: string }[]; value: string; onChange: (v: string) => void }) {
  return (
    <fieldset>
      <legend className="label">{label}</legend>
      <div className="flex flex-wrap gap-2">
        {options.map((o) => (
          <button key={o.value} type="button" onClick={() => onChange(o.value)} aria-pressed={value === o.value} className={value === o.value ? 'toggle toggle-on' : 'toggle'}>
            {o.label}
          </button>
        ))}
      </div>
    </fieldset>
  )
}
