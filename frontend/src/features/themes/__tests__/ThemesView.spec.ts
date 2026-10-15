import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ThemesView from '../ThemesView.vue'
import type { ThemePackage } from '../types'

const { listThemes, setActiveTheme, uploadTheme, applyTheme, suspendThemeRefresh, resumeRefresh, showSuccess } = vi.hoisted(() => ({
  listThemes: vi.fn(), setActiveTheme: vi.fn(), uploadTheme: vi.fn(), applyTheme: vi.fn(),
  suspendThemeRefresh: vi.fn(), resumeRefresh: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('../api', () => ({ listThemes, setActiveTheme, uploadTheme }))
vi.mock('../runtime', () => ({ applyTheme, suspendThemeRefresh, themeVariables: () => ({}) }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const ocean: ThemePackage = {
  id: 'a'.repeat(64), installed_at: '2026-10-15T00:00:00Z',
  manifest: {
    schema_version: 1, id: 'ocean', name: 'Ocean', version: '1.0.0', author: 'Sub2API',
    description: 'Ocean colors', tokens: { font: 'serif', radius_scale: 1.5 }
  }
}

const StepUpStub = defineComponent({
  props: ['controller'],
  template: '<button v-if="controller.visible.value" data-step-up @click="controller.onVerified()">Verify</button>'
})
const mountView = () => mount(ThemesView, {
  global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    Icon: true,
    TotpStepUpDialog: StepUpStub
  } }
})
let wrapper: ReturnType<typeof mountView> | undefined
const chooseFile = async (file: File) => {
  const input = wrapper!.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  listThemes.mockResolvedValue({ active_theme: null, themes: [ocean] })
  setActiveTheme.mockResolvedValue({ active_theme: ocean })
  uploadTheme.mockResolvedValue(ocean)
  suspendThemeRefresh.mockReturnValue(resumeRefresh)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

describe('theme management', () => {
  it('shows theme metadata and local previews without changing to an unselected theme', async () => {
    let finish!: (value: { active_theme: null, themes: ThemePackage[] }) => void
    listThemes.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    wrapper = mountView()
    await nextTick()
    expect(wrapper.get('[role="status"]').text()).toBe('common.loading')
    finish({ active_theme: null, themes: [ocean] })
    await flushPromises()
    const card = wrapper.get(`[data-theme-id="${ocean.id}"]`)
    expect(card.text()).toContain('Ocean')
    expect(card.text()).toContain('1.0.0')
    expect(card.text()).toContain('Sub2API')
    expect(card.text()).toContain('Ocean colors')
    expect(card.text()).toContain('1.5×')
    expect(card.findAll('[aria-label="admin.themes.preview"]')).toHaveLength(1)
    expect(applyTheme).toHaveBeenCalledTimes(1)
    expect(applyTheme).toHaveBeenCalledWith(null)
    expect(setActiveTheme).not.toHaveBeenCalled()
    expect(wrapper.get('a[download]').attributes('href')).toBe('/themes/examples/ocean.s2theme')
  })

  it('uploads without activating, adds the new card, and leaves the current theme untouched', async () => {
    listThemes.mockResolvedValue({ active_theme: null, themes: [] })
    wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('admin.themes.empty')
    applyTheme.mockClear()
    const file = new File(['zip'], 'ocean.s2theme')
    await chooseFile(file)
    expect(uploadTheme).toHaveBeenCalledWith(file)
    expect(wrapper.find(`[data-theme-id="${ocean.id}"]`).exists()).toBe(true)
    expect(setActiveTheme).not.toHaveBeenCalled()
    expect(applyTheme).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('admin.themes.uploadSuccess')
    expect(wrapper.get('[data-theme-id="default"] button').attributes('disabled')).toBeDefined()
  })

  it('applies a selected theme immediately and supports restoring defaults', async () => {
    wrapper = mountView()
    await flushPromises()
    applyTheme.mockClear()
    await wrapper.get(`[data-theme-id="${ocean.id}"] button`).trigger('click')
    await flushPromises()
    expect(setActiveTheme).toHaveBeenCalledWith(ocean.id)
    expect(suspendThemeRefresh).toHaveBeenCalledOnce()
    expect(applyTheme).toHaveBeenCalledWith(ocean)
    expect(resumeRefresh).toHaveBeenCalledOnce()
    expect(wrapper.get(`[data-theme-id="${ocean.id}"] button`).attributes('disabled')).toBeDefined()
    setActiveTheme.mockResolvedValue({ active_theme: null })
    await wrapper.get('[data-theme-id="default"] button').trigger('click')
    await flushPromises()
    expect(setActiveTheme).toHaveBeenLastCalledWith(null)
    expect(applyTheme).toHaveBeenLastCalledWith(null)
    expect(showSuccess).toHaveBeenLastCalledWith('admin.themes.defaultSuccess')
  })

  it('preserves the displayed theme if a save fails and shows an actionable error', async () => {
    setActiveTheme.mockRejectedValue({ message: 'Unable to save theme' })
    wrapper = mountView()
    await flushPromises()
    applyTheme.mockClear()
    await wrapper.get(`[data-theme-id="${ocean.id}"] button`).trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Unable to save theme')
    expect(applyTheme).not.toHaveBeenCalled()
    expect(resumeRefresh).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-theme-id="default"] button').attributes('disabled')).toBeDefined()
    expect(wrapper.get(`[data-theme-id="${ocean.id}"] button`).attributes('disabled')).toBeUndefined()
  })

  it('requires step-up verification before retrying a sensitive save', async () => {
    setActiveTheme.mockRejectedValueOnce({ code: 'STEP_UP_REQUIRED' }).mockResolvedValueOnce({ active_theme: ocean })
    wrapper = mountView()
    await flushPromises()
    await wrapper.get(`[data-theme-id="${ocean.id}"] button`).trigger('click')
    await flushPromises()
    expect(setActiveTheme).toHaveBeenCalledOnce()
    expect(wrapper.get(`[data-theme-id="${ocean.id}"] button`).attributes('disabled')).toBeDefined()
    await wrapper.get('[data-step-up]').trigger('click')
    await flushPromises()
    expect(setActiveTheme).toHaveBeenCalledTimes(2)
    expect(applyTheme).toHaveBeenLastCalledWith(ocean)
  })

  it('cancels the pending verification when navigating away', async () => {
    setActiveTheme.mockRejectedValue({ code: 'STEP_UP_REQUIRED' })
    wrapper = mountView()
    await flushPromises()
    await wrapper.get(`[data-theme-id="${ocean.id}"] button`).trigger('click')
    await flushPromises()
    wrapper.unmount()
    wrapper = undefined
    await flushPromises()
    expect(setActiveTheme).toHaveBeenCalledOnce()
    expect(resumeRefresh).toHaveBeenCalledOnce()
  })

  it('blocks concurrent uploads and saves while waiting for a server response', async () => {
    let finish!: (value: { active_theme: ThemePackage }) => void
    setActiveTheme.mockImplementation(() => new Promise((resolve) => { finish = resolve }))
    wrapper = mountView()
    await flushPromises()
    const button = wrapper.get(`[data-theme-id="${ocean.id}"] button`)
    await button.trigger('click')
    await button.trigger('click')
    await chooseFile(new File(['zip'], 'other.zip'))
    expect(setActiveTheme).toHaveBeenCalledOnce()
    expect(uploadTheme).not.toHaveBeenCalled()
    finish({ active_theme: ocean })
    await flushPromises()
    expect(applyTheme).toHaveBeenLastCalledWith(ocean)
  })

  it('recovers from a failed initial load and displays the empty state', async () => {
    listThemes.mockRejectedValueOnce({ message: 'Temporarily offline' }).mockResolvedValueOnce({ active_theme: null, themes: [] })
    wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Temporarily offline')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.themes.empty')
    expect(wrapper.find('[data-theme-id="default"]').exists()).toBe(true)
  })

  it('validates file type and size before calling the server', async () => {
    wrapper = mountView()
    await flushPromises()
    await chooseFile(new File(['text'], 'theme.js'))
    expect(wrapper.get('[role="alert"]').text()).toContain('admin.themes.fileRequired')
    await chooseFile(new File([new Uint8Array(256 * 1024 + 1)], 'theme.zip'))
    expect(wrapper.get('[role="alert"]').text()).toContain('admin.themes.fileTooLarge')
    expect(uploadTheme).not.toHaveBeenCalled()
  })
})
