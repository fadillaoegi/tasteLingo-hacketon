import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })

describe('request bounds', () => {
  it('aborts a stalled request and gives a retryable timeout message', async () => {
    vi.useFakeTimers()
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn((_url, init) => new Promise((_resolve, reject) => {
      signal = init.signal
      signal!.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
    })))
    const request = api.session('l1')
    const assertion = expect(request).rejects.toThrow('request took too long')
    await vi.advanceTimersByTimeAsync(20_001)
    await assertion
    expect(signal?.aborted).toBe(true)
  })

  it('forwards cancellation to the active plan request', async () => {
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn((_url, init) => new Promise((_resolve, reject) => {
      signal = init.signal
      signal!.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
    })))
    const controller = new AbortController()
    const request = api.buildPlan('l1', controller.signal)
    const assertion = expect(request).rejects.toMatchObject({ name: 'AbortError' })
    controller.abort()
    await assertion
    expect(signal?.aborted).toBe(true)
  })
})
