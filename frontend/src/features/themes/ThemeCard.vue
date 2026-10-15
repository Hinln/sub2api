<template>
  <article class="card flex flex-col border border-gray-200 dark:border-dark-700" :data-theme-id="theme?.id || 'default'">
    <div class="flex-1 p-5">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="min-w-0 break-words font-semibold text-gray-900 dark:text-white">
          {{ theme?.manifest.name || t('admin.themes.defaultName') }}
        </h3>
        <span v-if="active" class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
          {{ t('admin.themes.active') }}
        </span>
      </div>
      <p class="mt-1 break-words text-xs text-gray-500 dark:text-gray-400">
        <template v-if="theme">v{{ theme.manifest.version }}<span v-if="theme.manifest.author"> · {{ theme.manifest.author }}</span></template>
        <template v-else>{{ t('admin.themes.defaultAuthor') }}</template>
      </p>
      <p class="mt-3 min-h-10 break-words text-sm text-gray-600 dark:text-gray-300">
        {{ theme?.manifest.description || (theme ? t('admin.themes.noDescription') : t('admin.themes.defaultDescription')) }}
      </p>
      <ThemePreview class="mt-4" :theme="theme" />
      <dl class="mt-4 grid grid-cols-2 gap-3 text-xs">
        <div>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.themes.font') }}</dt>
          <dd class="mt-1 font-medium text-gray-700 dark:text-gray-200">{{ t(`admin.themes.fonts.${theme?.manifest.tokens.font || 'system'}`) }}</dd>
        </div>
        <div>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.themes.radius') }}</dt>
          <dd class="mt-1 font-medium text-gray-700 dark:text-gray-200">{{ theme?.manifest.tokens.radius_scale ?? 1 }}×</dd>
        </div>
      </dl>
    </div>
    <div class="flex justify-end border-t border-gray-100 px-5 py-4 dark:border-dark-700">
      <button type="button" class="btn btn-sm" :class="active ? 'btn-secondary' : 'btn-primary'" :disabled="disabled || active" @click="$emit('activate', theme)">
        {{ activating ? t('common.processing') : active ? t('admin.themes.active') : theme ? t('admin.themes.apply') : t('admin.themes.restoreDefault') }}
      </button>
    </div>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import ThemePreview from './ThemePreview.vue'
import type { ThemePackage } from './types'

defineProps<{
  theme: ThemePackage | null
  active: boolean
  disabled: boolean
  activating: boolean
}>()
defineEmits<{ activate: [theme: ThemePackage | null] }>()
const { t } = useI18n()
</script>
