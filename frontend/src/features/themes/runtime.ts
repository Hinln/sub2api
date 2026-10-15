import { buildApiUrl } from '@/api/url'
import { THEME_PALETTES, THEME_SHADES } from './types'
import type { ThemeFont, ThemePackage } from './types'

const REQUEST_TIMEOUT_MS = 1500
const FOCUS_REFRESH_INTERVAL_MS = 30000
const FONT_STACKS: Record<ThemeFont, string> = {
  system: 'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  serif: 'ui-serif, Georgia, Cambria, "Times New Roman", "Songti SC", SimSun, serif',
  mono: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace'
}
const RADIUS_BASES = { sm: 2, default: 4, md: 6, lg: 8, xl: 12, '2xl': 16, '3xl': 24, '4xl': 32 }
const VARIABLE_NAMES = [
  ...THEME_PALETTES.flatMap((palette) => THEME_SHADES.map((shade) => `--s2-${palette}-${shade}`)),
  ...Object.keys(RADIUS_BASES).map((size) => `--s2-radius-${size}`),
  '--s2-font-sans'
]

let revision = 0
let requestSequence = 0
let currentRequest: AbortController | null = null
let initialization: Promise<void> | null = null
let removeFocusListener: (() => void) | null = null
let lastRefresh = 0
let pendingChanges = 0

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** Build an allowlisted declaration set. Untrusted values can never become CSS code. */
export function themeVariables(theme: ThemePackage | null): Record<string, string> {
  const variables: Record<string, string> = {}
  if (!isRecord(theme) || !isRecord(theme.manifest) || theme.manifest.schema_version !== 1) return variables
  const tokens = theme.manifest.tokens
  if (!isRecord(tokens)) return variables
  if (isRecord(tokens.colors)) {
    for (const palette of THEME_PALETTES) {
      const colors = tokens.colors[palette]
      if (!isRecord(colors)) continue
      for (const shade of THEME_SHADES) {
        const color = colors[shade]
        if (typeof color !== 'string' || !/^#[\da-f]{6}$/i.test(color)) continue
        variables[`--s2-${palette}-${shade}`] = [1, 3, 5]
          .map((offset) => parseInt(color.slice(offset, offset + 2), 16))
          .join(' ')
      }
    }
  }
  if (typeof tokens.font === 'string' && Object.prototype.hasOwnProperty.call(FONT_STACKS, tokens.font)) {
    variables['--s2-font-sans'] = FONT_STACKS[tokens.font as ThemeFont]
  }
  const scale = tokens.radius_scale
  if (typeof scale === 'number' && Number.isFinite(scale) && scale >= 0.25 && scale <= 2) {
    for (const [size, base] of Object.entries(RADIUS_BASES)) {
      variables[`--s2-radius-${size}`] = `${base * scale}px`
    }
  }
  return variables
}

function invalidateRequests(): void {
  revision++
  currentRequest?.abort()
  currentRequest = null
}

/** Replace all theme overrides while preserving the user's light/dark mode. */
export function applyTheme(theme: ThemePackage | null): void {
  invalidateRequests()
  if (typeof document === 'undefined') return
  const style = document.documentElement.style
  for (const name of VARIABLE_NAMES) style.removeProperty(name)
  for (const [name, value] of Object.entries(themeVariables(theme))) style.setProperty(name, value)
}

/** Keep a public refresh from changing the page during an administrator's pending save. */
export function suspendThemeRefresh(): () => void {
  invalidateRequests()
  pendingChanges++
  let resumed = false
  return () => {
    if (resumed) return
    resumed = true
    pendingChanges = Math.max(0, pendingChanges - 1)
  }
}

function parseActiveTheme(payload: unknown): ThemePackage | null {
  let data = payload
  if (isRecord(data) && 'code' in data) {
    if (data.code !== 0) throw new Error('Theme request failed')
    data = data.data
  }
  if (!isRecord(data) || !Object.prototype.hasOwnProperty.call(data, 'active_theme')) throw new Error('Invalid theme response')
  if (data.active_theme === null) return null
  const theme = data.active_theme
  if (!isRecord(theme) || !isRecord(theme.manifest) || theme.manifest.schema_version !== 1 || !isRecord(theme.manifest.tokens)) {
    throw new Error('Invalid theme manifest')
  }
  return theme as unknown as ThemePackage
}

/** Public, credential-free request. Failure leaves the currently displayed theme intact. */
export async function refreshTheme(): Promise<void> {
  if (typeof window === 'undefined' || pendingChanges > 0) return
  currentRequest?.abort()
  const controller = new AbortController()
  currentRequest = controller
  const request = ++requestSequence
  const startedAtRevision = revision
  lastRefresh = Date.now()
  let timeout: ReturnType<typeof setTimeout> | undefined
  try {
    const response = await Promise.race([
      (async () => {
        const result = await fetch(buildApiUrl('/theme'), {
          signal: controller.signal,
          credentials: 'omit',
          cache: 'no-store',
          headers: { Accept: 'application/json' }
        })
        if (!result.ok) throw new Error('Theme request failed')
        return parseActiveTheme(await result.json())
      })(),
      new Promise<never>((_, reject) => {
        timeout = setTimeout(() => {
          controller.abort()
          reject(new Error('Theme request timed out'))
        }, REQUEST_TIMEOUT_MS)
      })
    ])
    if (request === requestSequence && startedAtRevision === revision && !controller.signal.aborted) {
      applyTheme(response)
    }
  } catch {
    // Availability must never prevent login or reset a theme after a failed save.
  } finally {
    if (timeout !== undefined) clearTimeout(timeout)
    if (currentRequest === controller) currentRequest = null
  }
}

/** Initialize once; callers can await it before mounting to avoid a flash of the default theme. */
export function initThemeLayer(): Promise<void> {
  if (initialization) return initialization
  if (typeof window === 'undefined') return Promise.resolve()
  const onFocus = () => {
    if (Date.now() - lastRefresh >= FOCUS_REFRESH_INTERVAL_MS) void refreshTheme()
  }
  window.addEventListener('focus', onFocus)
  removeFocusListener = () => window.removeEventListener('focus', onFocus)
  initialization = refreshTheme()
  return initialization
}

/** Also used by development hot reload to avoid duplicate listeners and stale requests. */
export function disposeThemeLayer(): void {
  invalidateRequests()
  removeFocusListener?.()
  removeFocusListener = null
  initialization = null
  lastRefresh = 0
  pendingChanges = 0
}

if (import.meta.hot) import.meta.hot.dispose(disposeThemeLayer)
