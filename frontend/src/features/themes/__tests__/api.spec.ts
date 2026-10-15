import { beforeEach, describe, expect, it, vi } from 'vitest'
import { listThemes, setActiveTheme, uploadTheme } from '../api'

const { get, post, put } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, post, put } }))
beforeEach(() => vi.clearAllMocks())

describe('theme administration API', () => {
  it('fetches installed themes', async () => {
    const themes = { active_theme: null, themes: [] }
    get.mockResolvedValue({ data: themes })
    await expect(listThemes()).resolves.toBe(themes)
    expect(get).toHaveBeenCalledWith('/admin/themes')
  })

  it('sends the package in multipart form without imposing a boundary or JSON content type', async () => {
    const file = new File(['zip'], 'ocean.s2theme')
    const theme = { id: 'a'.repeat(64) }
    post.mockResolvedValue({ data: theme })
    await expect(uploadTheme(file)).resolves.toBe(theme)
    const [url, form, config] = post.mock.calls[0]
    expect(url).toBe('/admin/themes/upload')
    expect(form).toBeInstanceOf(FormData)
    expect(form.get('theme')).toBe(file)
    expect(config.headers['Content-Type']).toBeUndefined()
  })

  it.each(['a'.repeat(64), null])('selects the theme or the default (%s)', async (id) => {
    put.mockResolvedValue({ data: { active_theme: null } })
    await setActiveTheme(id)
    expect(put).toHaveBeenCalledWith('/admin/themes/active', { theme_id: id })
  })
})
