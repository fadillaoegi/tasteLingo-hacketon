import { BookOpen, Film, Mic, Music, Tv } from 'lucide-react'
import type { EntityType } from '../api'

const icons = { movie: Film, tv_show: Tv, book: BookOpen, podcast: Mic, artist: Music }

export function TypeIcon({ type, small }: { type: EntityType; small?: boolean }) {
  const Icon = icons[type] ?? Film
  return <Icon className={small ? 'size-3.5 shrink-0 text-teal-700' : 'size-5 shrink-0 text-teal-700'} aria-hidden />
}
