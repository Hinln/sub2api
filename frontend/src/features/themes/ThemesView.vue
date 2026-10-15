<template>
  <AppLayout>
    <div class="space-y-6">
      <section class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.themes.title') }}</h2>
          <p class="mt-1 max-w-2xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.themes.description') }}</p>
        </div>
        <div class="flex flex-shrink-0 items-center gap-2">
          <input ref="fileInput" type="file" class="hidden" accept=".s2theme,.zip,application/zip" :disabled="busy" :aria-label="t('admin.themes.upload')" @change="handleFileSelected" />
          <button type="button" class="btn btn-primary" :disabled="busy || loading" @click="fileInput?.click()">
            <Icon name="upload" size="sm" />
            {{ uploading ? t('common.processing') : t('admin.themes.upload') }}
          </button>
          <button type="button" class="btn btn-secondary" :disabled="busy || loading" :aria-label="t('common.refresh')" @click="loadThemes">
            <Icon name="refresh" size="sm" />
          </button>
        </div>
      </section>

      <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500 dark:text-gray-400">
        <p>{{ t('admin.themes.uploadHint') }}</p>
        <a href="/themes/examples/ocean.s2theme" download="ocean.s2theme" class="font-medium text-primary-600 hover:underline dark:text-primary-400">
          {{ t('admin.themes.downloadExample') }}
        </a>
      </div>

      <div v-if="error" role="alert" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/20 dark:text-red-300">
        <p class="break-words">{{ error }}</p>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || busy" @click="loadThemes">{{ t('admin.themes.retry') }}</button>
      </div>

      <div v-if="loading && !loaded" role="status" class="flex min-h-56 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>

      <template v-if="loaded">
        <section class="rounded-xl border border-primary-200 bg-primary-50 px-5 py-4 dark:border-primary-900/60 dark:bg-primary-900/10" aria-live="polite">
          <p class="text-xs font-medium text-primary-700 dark:text-primary-300">{{ t('admin.themes.currentTheme') }}</p>
          <p class="mt-1 break-words text-base font-semibold text-gray-900 dark:text-white">{{ activeTheme?.manifest.name || t('admin.themes.defaultName') }}</p>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-300">{{ t('admin.themes.scopeHint') }}</p>
        </section>

        <section :aria-label="t('admin.themes.available')" :aria-busy="loading || busy">
          <h3 class="mb-4 text-sm font-semibold text-gray-800 dark:text-gray-200">{{ t('admin.themes.available') }}</h3>
          <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <ThemeCard :theme="null" :active="activeTheme === null" :disabled="busy || loading" :activating="switchingID === 'default'" @activate="activateTheme" />
            <ThemeCard v-for="theme in themes" :key="theme.id" :theme="theme" :active="theme.id === activeTheme?.id" :disabled="busy || loading" :activating="switchingID === theme.id" @activate="activateTheme" />
          </div>
          <div v-if="themes.length === 0" class="mt-4 rounded-xl border border-dashed border-gray-300 p-6 text-center dark:border-dark-600">
            <p class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.themes.empty') }}</p>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.themes.emptyHint') }}</p>
          </div>
        </section>
      </template>
      <TotpStepUpDialog :controller="stepUp" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { listThemes, setActiveTheme, uploadTheme } from './api'
import { applyTheme, suspendThemeRefresh } from './runtime'
import ThemeCard from './ThemeCard.vue'
import type { ThemePackage } from './types'

const MAX_PACKAGE_BYTES = 256 * 1024
const { t } = useI18n()
const appStore = useAppStore()
const stepUp = useStepUp()
const themes = ref<ThemePackage[]>([])
const activeTheme = ref<ThemePackage | null>(null)
const loading = ref(false)
const loaded = ref(false)
const uploading = ref(false)
const switchingID = ref<string | null>(null)
const error = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const busy = computed(() => uploading.value || switchingID.value !== null)
let disposed = false
let listSequence = 0

function showActionError(cause: unknown): void {
  if (isStepUpCancelled(cause)) return
  if (isStepUpBlocked(cause)) {
    error.value = stepUpBlockReason(cause) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
      ? t('stepUp.adminApiKeyForbidden')
      : t('stepUp.notEnabled')
  } else {
    error.value = typeof cause === 'object' && cause !== null && 'message' in cause && typeof cause.message === 'string'
      ? cause.message
      : t('admin.themes.operationFailed')
  }
}

async function loadThemes(): Promise<void> {
  if (busy.value || loading.value) return
  loading.value = true
  error.value = ''
  const sequence = ++listSequence
  try {
    const result = await listThemes()
    if (disposed || sequence !== listSequence) return
    themes.value = result.themes
    activeTheme.value = result.active_theme
    applyTheme(result.active_theme)
    loaded.value = true
  } catch (cause) {
    if (!disposed && sequence === listSequence) showActionError(cause)
  } finally {
    if (!disposed && sequence === listSequence) loading.value = false
  }
}

async function handleFileSelected(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || busy.value || loading.value) return
  if (!/\.(s2theme|zip)$/i.test(file.name)) {
    error.value = t('admin.themes.fileRequired')
    return
  }
  if (file.size > MAX_PACKAGE_BYTES) {
    error.value = t('admin.themes.fileTooLarge')
    return
  }
  uploading.value = true
  error.value = ''
  try {
    const theme = await stepUp.run(() => uploadTheme(file))
    if (disposed) return
    themes.value = [theme, ...themes.value.filter((installed) => installed.id !== theme.id)]
    appStore.showSuccess(t('admin.themes.uploadSuccess'))
  } catch (cause) {
    if (!disposed) showActionError(cause)
  } finally {
    uploading.value = false
  }
}

async function activateTheme(theme: ThemePackage | null): Promise<void> {
  if (busy.value || loading.value || theme?.id === activeTheme.value?.id) return
  switchingID.value = theme?.id || 'default'
  error.value = ''
  const resumeRefresh = suspendThemeRefresh()
  try {
    const result = await stepUp.run(() => setActiveTheme(theme?.id || null))
    // A completed server change still applies if navigation happened while it was saving.
    applyTheme(result.active_theme)
    if (disposed) return
    activeTheme.value = result.active_theme
    appStore.showSuccess(t(theme ? 'admin.themes.applySuccess' : 'admin.themes.defaultSuccess'))
  } catch (cause) {
    if (!disposed) showActionError(cause)
  } finally {
    resumeRefresh()
    switchingID.value = null
  }
}

onMounted(loadThemes)
onUnmounted(() => {
  disposed = true
  listSequence++
  stepUp.onCancel()
})
</script>
