import type { RouteRecordRaw } from 'vue-router'

// Keep the extension route separate from upstream's main route definitions.
export const themeRoutes: RouteRecordRaw[] = [
  {
    path: '/admin/themes',
    name: 'AdminThemes',
    component: () => import('./ThemesView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Theme Management',
      titleKey: 'admin.themes.title',
      descriptionKey: 'admin.themes.description'
    }
  }
]
