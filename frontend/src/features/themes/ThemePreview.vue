<template>
  <div class="grid grid-cols-2 gap-3" :style="{ fontFamily: font }" :aria-label="t('admin.themes.preview')">
    <div
      v-for="mode in modes"
      :key="mode.label"
      class="min-w-0 border p-3"
      :style="{ backgroundColor: mode.background, color: mode.foreground, borderColor: mode.border, borderRadius: radius }"
    >
      <p class="mb-3 text-xs font-medium opacity-70">{{ mode.label }}</p>
      <div class="mb-2 h-1.5 w-10 rounded-full opacity-30" :style="{ backgroundColor: mode.foreground }"></div>
      <p class="truncate text-sm font-semibold">{{ t('admin.themes.previewHeading') }}</p>
      <span class="mt-3 inline-block px-3 py-1.5 text-xs font-medium text-white" :style="{ backgroundColor: primary, borderRadius: radius }">
        {{ t('admin.themes.previewButton') }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { themeVariables } from './runtime'
import type { ThemePackage } from './types'

const props = defineProps<{ theme: ThemePackage | null }>()
const { t } = useI18n()
const variables = computed(() => themeVariables(props.theme))
const color = (name: string, fallback: string) => `rgb(${variables.value[`--s2-${name}`] || fallback})`
const font = computed(() => variables.value['--s2-font-sans'] || 'system-ui, -apple-system, sans-serif')
const radius = computed(() => variables.value['--s2-radius-lg'] || '8px')
const primary = computed(() => color('primary-600', '13 148 136'))
const modes = computed(() => [
  {
    label: t('admin.themes.previewLight'),
    background: color('gray-50', '249 250 251'),
    foreground: color('gray-900', '17 24 39'),
    border: color('gray-200', '229 231 235')
  },
  {
    label: t('admin.themes.previewDark'),
    background: color('dark-800', '30 41 59'),
    foreground: color('gray-100', '243 244 246'),
    border: color('dark-700', '51 65 85')
  }
])
</script>
