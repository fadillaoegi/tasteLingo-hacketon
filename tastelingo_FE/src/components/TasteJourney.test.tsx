// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { TasteJourney } from './TasteJourney'

let container: HTMLDivElement
let root: Root
beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.unstubAllGlobals() })

it('explains the selected stage and lets the user pause decorative motion', async () => {
  const demo = vi.fn()
  await act(async () => root.render(<TasteJourney favorites={[]} level="B1" daily={30} onDemo={demo} busy={false} />))
  const steps = container.querySelectorAll<HTMLButtonElement>('.journey-stage-buttons button')
  await act(async () => steps[1].click())
  expect(container.querySelector('.taste-scene')?.getAttribute('data-stage')).toBe('1')
  expect(container.querySelector('#journey-explanation')?.textContent).toContain('Discover what connects')
  expect(steps[1].getAttribute('aria-pressed')).toBe('true')
  await act(async () => container.querySelector<HTMLButtonElement>('.motion-control')!.click())
  expect(container.querySelector('.taste-intro')?.getAttribute('data-motion')).toBe('paused')
  expect(demo).not.toHaveBeenCalled()
})

it('uses real selected preferences and respects reduced motion', async () => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  await act(async () => root.render(<TasteJourney favorites={[{ id: 'synthetic-film', name: 'My chosen film', type: 'movie', source: 'user' }]} level="A2" daily={15} onDemo={vi.fn()} busy={false} />))
  expect(container.textContent).toContain('My chosen film')
  expect(container.textContent).toContain('1 favourite shaping your path')
  expect(container.textContent).toContain('15 min a day')
  expect(container.querySelector('.taste-intro')?.getAttribute('data-motion')).toBe('paused')
  expect(container.querySelector<HTMLButtonElement>('.motion-control')?.disabled).toBe(true)
})
