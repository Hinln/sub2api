import assert from 'node:assert/strict'
import { test } from 'node:test'
import postcss from 'postcss'
import tailwindcss from 'tailwindcss'
import config from '../tailwind.config.js'
import { withThemeTokens } from './tailwind.mjs'

test('theme adapter preserves upstream configuration and original fallbacks', () => {
  const base = {
    content: ['a.vue'],
    plugins: [],
    theme: { extend: {
      colors: { primary: { 500: '#14b8a6' }, warning: '#abcdef' },
      borderRadius: { '4xl': '2rem' },
      fontFamily: { sans: ['system-ui', 'Arial'] },
      boxShadow: { glow: '0 0 20px rgba(20, 184, 166, 0.25)' },
      backgroundImage: { brand: 'linear-gradient(135deg, #14b8a6, #000000)' },
      keyframes: { glow: { from: { boxShadow: '0 0 30px rgba(20,184,166,0.4)' } } }
    } }
  }
  const original = structuredClone(base)
  const adapted = withThemeTokens(base)
  assert.deepEqual(base, original)
  assert.equal(adapted.plugins, base.plugins)
  assert.equal(adapted.theme.extend.colors.warning, '#abcdef')
  assert.equal(adapted.theme.extend.colors.primary[500], 'rgb(var(--s2-primary-500, 20 184 166) / <alpha-value>)')
  assert.equal(adapted.theme.extend.colors.gray[50], 'rgb(var(--s2-gray-50, 249 250 251) / <alpha-value>)')
  assert.equal(adapted.theme.extend.borderRadius['4xl'], 'var(--s2-radius-4xl, 2rem)')
  assert.match(adapted.theme.extend.boxShadow.glow, /var\(--s2-primary-500, 20 184 166\) \/ 0\.25/)
  assert.match(adapted.theme.extend.keyframes.glow.from.boxShadow, /var\(--s2-primary-500, 20 184 166\) \/ 0\.4/)
  assert.match(adapted.theme.extend.backgroundImage.brand, /#000000/)
})

test('real Tailwind CSS preserves opacity modifiers, dark variants, typography and radii', async () => {
  const result = await postcss([tailwindcss({
    ...config,
    content: [{ raw: 'bg-primary-500 bg-primary-500/20 dark:bg-dark-950 text-gray-900 rounded-xl rounded-full font-sans bg-gradient-primary shadow-glow', extension: 'html' }]
  })]).process('@tailwind base; @tailwind utilities;', { from: undefined })
  const declarations = selector => {
    const values = []
    result.root.walkRules(rule => {
      if (rule.selector === selector) rule.walkDecls(decl => values.push(`${decl.prop}: ${decl.value}`))
    })
    return values.join('; ')
  }
  assert.match(declarations('.bg-primary-500'), /var\(--s2-primary-500, 20 184 166\)/)
  assert.match(declarations('.bg-primary-500\\/20'), /\/ 0\.2\)/)
  assert.match(result.css, /--s2-dark-950, 2 6 23/)
  assert.match(declarations('.text-gray-900'), /--s2-gray-900, 17 24 39/)
  assert.match(declarations('.rounded-xl'), /var\(--s2-radius-xl, 0\.75rem\)/)
  assert.match(declarations('.rounded-full'), /9999px/)
  assert.match(declarations('.font-sans'), /var\(--s2-font-sans, system-ui/)
  assert.match(declarations('.bg-gradient-primary'), /var\(--s2-primary-500/)
  assert.match(declarations('.shadow-glow'), /var\(--s2-primary-500/)
})
