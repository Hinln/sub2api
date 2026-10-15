import colors from 'tailwindcss/colors.js'
import defaultTheme from 'tailwindcss/defaultTheme.js'

const palettes = ['primary', 'gray', 'dark', 'accent']
const radii = ['sm', 'DEFAULT', 'md', 'lg', 'xl', '2xl', '3xl', '4xl']

function rgb(hex) {
  if (typeof hex !== 'string' || !/^#[\da-f]{6}$/i.test(hex)) return null
  return [1, 3, 5].map(start => parseInt(hex.slice(start, start + 2), 16)).join(' ')
}

// Keep upstream values as CSS fallbacks. Installing this hook without activating
// a theme must preserve the existing design, including opacity modifiers.
export function withThemeTokens(config) {
  const extend = config.theme?.extend ?? {}
  const originalColors = { ...extend.colors, gray: extend.colors?.gray ?? colors.gray }
  const themedColors = { ...extend.colors }
  const colorTokens = new Map()
  for (const palette of palettes) {
    const original = originalColors[palette]
    if (!original) continue
    themedColors[palette] = Object.fromEntries(Object.entries(original).map(([shade, value]) => {
      const tuple = rgb(value)
      if (!tuple) return [shade, value]
      const variable = `--s2-${palette}-${shade}`
      // Prefer dark over accent for identical slate defaults in gradient-dark.
      if (!colorTokens.has(tuple)) colorTokens.set(tuple, variable)
      return [shade, `rgb(var(${variable}, ${tuple}) / <alpha-value>)`]
    }))
  }

  // Upstream also uses literal brand colors in gradients and glow effects.
  // Rebind those values here instead of patching individual Vue components.
  function effect(value) {
    if (Array.isArray(value)) return value.map(effect)
    if (value && typeof value === 'object') {
      return Object.fromEntries(Object.entries(value).map(([key, child]) => [key, effect(child)]))
    }
    if (typeof value !== 'string') return value
    return value.replace(/#[\da-f]{6}\b|rgba?\(\s*\d+\s*,\s*\d+\s*,\s*\d+(?:\s*,\s*[\d.]+)?\s*\)/gi, match => {
      const parts = match.startsWith('#') ? null : match.match(/[\d.]+/g)
      const tuple = parts ? parts.slice(0, 3).map(Number).join(' ') : rgb(match)
      const variable = colorTokens.get(tuple)
      if (!variable) return match
      const alpha = parts?.[3] ?? '1'
      return `rgb(var(${variable}, ${tuple}) / ${alpha})`
    })
  }

  const borderRadius = { ...extend.borderRadius }
  for (const key of radii) {
    const fallback = extend.borderRadius?.[key] ?? defaultTheme.borderRadius[key]
    if (fallback) borderRadius[key] = `var(--s2-radius-${key === 'DEFAULT' ? 'default' : key}, ${fallback})`
  }
  const sans = extend.fontFamily?.sans ?? defaultTheme.fontFamily.sans
  const stack = Array.isArray(sans) ? sans.join(', ') : sans
  return {
    ...config,
    theme: {
      ...config.theme,
      extend: {
        ...extend,
        colors: themedColors,
        borderRadius,
        fontFamily: { ...extend.fontFamily, sans: [`var(--s2-font-sans, ${stack})`] },
        ...(extend.boxShadow ? { boxShadow: effect(extend.boxShadow) } : {}),
        ...(extend.backgroundImage ? { backgroundImage: effect(extend.backgroundImage) } : {}),
        ...(extend.keyframes ? { keyframes: effect(extend.keyframes) } : {})
      }
    }
  }
}
