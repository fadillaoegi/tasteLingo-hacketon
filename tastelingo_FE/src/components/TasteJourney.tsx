import { useEffect, useRef, useState, type CSSProperties, type PointerEvent } from 'react'
import { ArrowDown, ArrowRight, BookOpen, Check, Film, Headphones, Music2, Network, Pause, Play, Sparkles } from 'lucide-react'
import type { Entity } from '../api'

const stages = [
  { label: 'Your taste', title: 'Start with something you love.', text: 'Choose a favourite film, book, artist or podcast. A little of your personality is all we need to start.', icon: Film },
  { label: 'The connections', title: 'Discover what connects your interests.', text: 'The taste graph finds related content across films, books, music and podcasts, giving your learning plan a personal starting point.', icon: Network },
  { label: 'Your learning', title: 'Turn discovery into a daily habit.', text: 'Your interests become a 7-day English plan, with activities shaped around your level, goal and available time.', icon: BookOpen },
]

interface Props {
  favorites: Entity[]
  level: string
  daily: number
  onDemo: () => void
  busy: boolean
}

export function TasteJourney({ favorites, level, daily, onDemo, busy }: Props) {
  const [stage, setStage] = useState(0)
  const [paused, setPaused] = useState(false)
  const [reduced, setReduced] = useState(false)
  const scene = useRef<HTMLDivElement>(null)
  const frame = useRef<number | null>(null)
  const still = paused || reduced

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const query = window.matchMedia('(prefers-reduced-motion: reduce)')
    const update = () => setReduced(query.matches)
    update()
    query.addEventListener('change', update)
    return () => query.removeEventListener('change', update)
  }, [])

  useEffect(() => () => { if (frame.current !== null) cancelAnimationFrame(frame.current) }, [])
  useEffect(() => {
    if (!still) return
    scene.current?.style.setProperty('--scene-rx', '0deg')
    scene.current?.style.setProperty('--scene-ry', '0deg')
  }, [still])

  function tilt(event: PointerEvent<HTMLDivElement>) {
    if (still || event.pointerType === 'touch' || typeof requestAnimationFrame !== 'function') return
    const rect = event.currentTarget.getBoundingClientRect()
    const x = (event.clientX - rect.left) / rect.width - 0.5
    const y = (event.clientY - rect.top) / rect.height - 0.5
    if (frame.current !== null) cancelAnimationFrame(frame.current)
    frame.current = requestAnimationFrame(() => {
      scene.current?.style.setProperty('--scene-rx', `${-y * 7}deg`)
      scene.current?.style.setProperty('--scene-ry', `${x * 9}deg`)
    })
  }

  function untilt() {
    if (frame.current !== null) cancelAnimationFrame(frame.current)
    scene.current?.style.setProperty('--scene-rx', '0deg')
    scene.current?.style.setProperty('--scene-ry', '0deg')
  }

  const current = stages[stage]
  const StageIcon = current.icon
  const film = favorites.find(f => f.type === 'movie' || f.type === 'tv_show')
  const music = favorites.find(f => f.type === 'artist')
  const reading = favorites.find(f => f.type === 'book' || f.type === 'podcast')

  return (
    <section className="taste-intro" data-motion={still ? 'paused' : 'playing'} aria-labelledby="intro-title">
      <div className="intro-copy">
        <p className="eyebrow"><span className="eyebrow-dot" /> A learning plan with your personality</p>
        <h1 id="intro-title">English, made of<br /><span>what you love.</span></h1>
        <p className="intro-description">Your favourite stories, sounds and ideas are a great place to start. Turn them into a personal 7-day English learning plan.</p>
        <div className="intro-actions">
          <button className="btn-primary" onClick={onDemo} disabled={busy}>Try the demo <ArrowRight className="size-4" /></button>
          <a className="intro-secondary" href="#personalise-form">Make it personal <ArrowDown className="size-4" /></a>
        </div>
        <p className="intro-demo-note">Meet Rina: B1 English, big interests, 30 minutes a day.</p>
        <div className="intro-principles">
          <span><Check className="size-3.5" /> Your interests</span>
          <span><Check className="size-3.5" /> Your level</span>
          <span><Check className="size-3.5" /> Your pace</span>
        </div>
      </div>

      <div className="journey-visual" onPointerMove={tilt} onPointerLeave={untilt}>
        <div className="scene-topline"><span>THE TASTE STUDIO</span><button className="motion-control" onClick={() => setPaused(!paused)} disabled={reduced} aria-pressed={still} aria-label={reduced ? 'Reduced motion enabled' : paused ? 'Play motion' : 'Pause motion'}>{still ? <Play size={12} /> : <Pause size={12} />}{reduced ? 'Reduced motion' : paused ? 'Play motion' : 'Pause motion'}</button></div>
        <div ref={scene} className="taste-scene" data-stage={stage} aria-label="Illustration: personal interests connect to a learning plan">
          <div className="scene-grid" aria-hidden="true" />
          <div className="scene-orbit scene-orbit-one" aria-hidden="true"><i /></div>
          <div className="scene-orbit scene-orbit-two" aria-hidden="true"><i /></div>
          <div className="floating-interest interest-film">
            <span className="interest-icon"><Film size={22} /></span>
            <span className="interest-type">STORIES YOU LOVE</span>
            <strong>{film?.name ?? 'That favourite film'}</strong>
            <span className="interest-footnote">A spark for discovery</span>
          </div>
          <div className="floating-interest interest-music">
            <span className="interest-icon"><Music2 size={21} /></span>
            <span className="interest-type">YOUR KIND OF SOUND</span>
            <strong>{music?.name ?? 'Music on repeat'}</strong>
            <span className="sound-bars" aria-hidden="true">{Array.from({ length: 7 }, (_, i) => <i key={i} style={{ '--bar-index': i } as CSSProperties} />)}</span>
          </div>
          <div className="floating-interest interest-reading">
            <span className="interest-icon">{reading?.type === 'podcast' ? <Headphones size={20} /> : <BookOpen size={20} />}</span>
            <span><span className="interest-type">IDEAS THAT STAY</span><strong>{reading?.name ?? 'One more chapter'}</strong></span>
          </div>
          <div className="scene-core" aria-hidden="true">
            <div className="core-halo" />
            <img src={`${import.meta.env.BASE_URL}brand/tastelingo-mark-3d.webp`} alt="" width="512" height="512" />
            <span className="core-plinth" />
          </div>
          <div className="scene-plan">
            <span className="plan-preview-heading"><span><Sparkles size={13} /> YOUR LEARNING PATH</span><span>{level}</span></span>
            <strong>A little English, every day.</strong>
            <div className="preview-week" aria-hidden="true">{['M', 'T', 'W', 'T', 'F', 'S', 'S'].map((day, i) => <span key={i} style={{ '--day-index': i } as CSSProperties}>{day}<i /></span>)}</div>
            <span className="preview-meta">7 days <i /> {daily} min a day <i /> Made for you</span>
          </div>
        </div>
        <p className="scene-annotation"><span /> {favorites.length ? `${favorites.length} favourite${favorites.length === 1 ? '' : 's'} shaping your path` : 'Your interests are the starting point'}</p>
      </div>

      <div className="journey-explainer" id="how-it-works">
        <div className="journey-stage-buttons" role="group" aria-label="Explore how TasteLingo works">
          {stages.map((step, index) => <button key={step.label} onClick={() => setStage(index)} aria-pressed={stage === index} aria-controls="journey-explanation"><span>0{index + 1}</span>{step.label}<ArrowRight size={14} /></button>)}
        </div>
        <div className="journey-explanation" id="journey-explanation" aria-live="polite" aria-atomic="true">
          <span className="explanation-icon"><StageIcon size={21} /></span>
          <div><h2>{current.title}</h2><p>{current.text}</p></div>
        </div>
      </div>
    </section>
  )
}
