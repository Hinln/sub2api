import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { applyTheme, disposeThemeLayer, initThemeLayer, refreshTheme, suspendThemeRefresh, themeVariables } from '../runtime'
import type { ThemePackage } from '../types'

const ocean: ThemePackage = {
  id: 'a'.repeat(64),
  installed_at: '2026-10-15T00:00:00Z',
  manifest: {
    schema_version: 1, id: 'ocean', name: 'Ocean', version: '1.0.0',
    tokens: {
      colors: { primary: { '500': '#123456', '600': '#023047' }, gray: { '50': '#fcfcfc' } },
      font: 'serif', radius_scale: 1.5
    }
  }
}
const response = (theme: ThemePackage | null) => ({ ok: true, json: async () => ({ code: 0, data: { active_theme: theme } }) })
const style = () => document.documentElement.style
const deferred = <T>() => {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

beforeEach(() => {
  vi.useFakeTimers()
  disposeThemeLayer()
  applyTheme(null)
  document.documentElement.classList.add('dark')
})
afterEach(() => {
  disposeThemeLayer()
  applyTheme(null)
  document.documentElement.classList.remove('dark')
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('theme runtime', () => {
  it('loads before mount, uses the API base URL, and sends no authentication credentials', async () => {
    const fetcher = vi.fn().mockResolvedValue(response(ocean))
    vi.stubGlobal('fetch', fetcher)
    localStorage.setItem('auth_token', 'must-not-be-sent')
    const storageSpy = vi.spyOn(Storage.prototype, 'getItem')
    await initThemeLayer()
    expect(fetcher).toHaveBeenCalledWith('/api/v1/theme', expect.objectContaining({ credentials: 'omit', cache: 'no-store' }))
    expect(fetcher.mock.calls[0][1].headers).toEqual({ Accept: 'application/json' })
    expect(storageSpy).not.toHaveBeenCalled()
    expect(style().getPropertyValue('--s2-primary-500')).toBe('18 52 86')
    expect(style().getPropertyValue('--s2-radius-lg')).toBe('12px')
    expect(style().getPropertyValue('--s2-font-sans')).toContain('ui-serif')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    localStorage.removeItem('auth_token')
  })

  it('clears every old override on switching and restores defaults without losing dark mode', () => {
    style().setProperty('--unrelated', 'keep')
    applyTheme(ocean)
    applyTheme({ ...ocean, manifest: { ...ocean.manifest, tokens: { colors: { primary: { '600': '#abcdef' } } } } })
    expect(style().getPropertyValue('--s2-primary-600')).toBe('171 205 239')
    expect(style().getPropertyValue('--s2-primary-500')).toBe('')
    expect(style().getPropertyValue('--s2-gray-50')).toBe('')
    expect(style().getPropertyValue('--s2-radius-lg')).toBe('')
    expect(style().getPropertyValue('--s2-font-sans')).toBe('')
    applyTheme(null)
    expect(style().getPropertyValue('--s2-primary-600')).toBe('')
    expect(style().getPropertyValue('--unrelated')).toBe('keep')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    style().removeProperty('--unrelated')
  })

  it('ignores malformed or executable token values even if the server returns them', () => {
    const unsafe = {
      ...ocean,
      manifest: { ...ocean.manifest, tokens: {
        colors: {
          primary: { '500': '#12ff12', '600': 'red; background: url(https://invalid.test)', '999': '#ffffff' },
          injected: { '500': '#ffffff' }
        },
        font: 'url(https://invalid.test)', radius_scale: Number.POSITIVE_INFINITY,
        css: 'body { display: none }'
      } }
    } as unknown as ThemePackage
    expect(themeVariables(unsafe)).toEqual({ '--s2-primary-500': '18 255 18' })
    expect(themeVariables({ ...ocean, manifest: { ...ocean.manifest, schema_version: 2 } } as unknown as ThemePackage)).toEqual({})
    for (const radius_scale of [-1, 0, 2.1, Number.NaN]) {
      expect(themeVariables({ ...ocean, manifest: { ...ocean.manifest, tokens: { radius_scale } } })).toEqual({})
    }
  })

  it('finishes within 1500ms even when fetch ignores abort', async () => {
    const late = deferred<ReturnType<typeof response>>()
    vi.stubGlobal('fetch', vi.fn(() => late.promise))
    let finished = false
    const initial = initThemeLayer().then(() => { finished = true })
    await vi.advanceTimersByTimeAsync(1499)
    expect(finished).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    await initial
    expect(finished).toBe(true)
    expect(style().getPropertyValue('--s2-primary-500')).toBe('')
    late.resolve(response(ocean))
    await Promise.resolve()
    expect(style().getPropertyValue('--s2-primary-500')).toBe('')
  })

  it('also bounds a stalled response body and keeps the active theme after failures', async () => {
    applyTheme(ocean)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: () => new Promise(() => {}) }))
    const refresh = refreshTheme()
    await vi.advanceTimersByTimeAsync(1500)
    await refresh
    expect(style().getPropertyValue('--s2-primary-500')).toBe('18 52 86')
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
    await refreshTheme()
    expect(style().getPropertyValue('--s2-primary-500')).toBe('18 52 86')
  })

  it('never lets an earlier public request replace a newer administrator selection', async () => {
    const stale = deferred<ReturnType<typeof response>>()
    vi.stubGlobal('fetch', vi.fn(() => stale.promise))
    const initial = initThemeLayer()
    applyTheme(ocean)
    stale.resolve(response(null))
    await initial
    expect(style().getPropertyValue('--s2-primary-500')).toBe('18 52 86')
  })

  it('only applies the latest public request and supports unwrapped responses', async () => {
    const stale = deferred<ReturnType<typeof response>>()
    const fetcher = vi.fn().mockReturnValueOnce(stale.promise).mockResolvedValueOnce({ ok: true, json: async () => ({ active_theme: ocean }) })
    vi.stubGlobal('fetch', fetcher)
    const initial = initThemeLayer()
    await refreshTheme()
    stale.resolve(response(null))
    await initial
    expect(style().getPropertyValue('--s2-primary-500')).toBe('18 52 86')
  })

  it('initializes idempotently, throttles focus refreshes, and removes listeners on disposal', async () => {
    const fetcher = vi.fn().mockResolvedValue(response(null))
    vi.stubGlobal('fetch', fetcher)
    const first = initThemeLayer()
    expect(initThemeLayer()).toBe(first)
    await first
    window.dispatchEvent(new Event('focus'))
    expect(fetcher).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(30000)
    window.dispatchEvent(new Event('focus'))
    await Promise.resolve()
    expect(fetcher).toHaveBeenCalledTimes(2)
    disposeThemeLayer()
    await vi.advanceTimersByTimeAsync(30000)
    window.dispatchEvent(new Event('focus'))
    expect(fetcher).toHaveBeenCalledTimes(2)
    await initThemeLayer()
    expect(fetcher).toHaveBeenCalledTimes(3)
  })

  it('holds the displayed theme during a pending save and resumes refreshes afterward', async () => {
    applyTheme(ocean)
    const fetcher = vi.fn().mockResolvedValue(response(null))
    vi.stubGlobal('fetch', fetcher)
    const resume = suspendThemeRefresh()
    await refreshTheme()
    expect(fetcher).not.toHaveBeenCalled()
    expect(style().getPropertyValue('--s2-primary-500')).toBe('18 52 86')
    resume()
    resume()
    await refreshTheme()
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(style().getPropertyValue('--s2-primary-500')).toBe('')
  })

  it.each([
    { ok: false, json: async () => ({}) },
    { ok: true, json: async () => ({ code: 1, data: { active_theme: null } }) },
    { ok: true, json: async () => ({}) },
    { ok: true, json: async () => ({ active_theme: { manifest: { schema_version: 2 } } }) }
  ])('ignores failed and malformed public responses', async (result) => {
    applyTheme(ocean)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(result))
    await refreshTheme()
    expect(style().getPropertyValue('--s2-primary-500')).toBe('18 52 86')
  })
})
