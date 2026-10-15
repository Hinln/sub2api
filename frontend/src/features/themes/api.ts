import { apiClient } from '@/api/client'
import type { ActiveThemeResponse, ThemeListResponse, ThemePackage } from './types'

export async function listThemes(): Promise<ThemeListResponse> {
  const { data } = await apiClient.get<ThemeListResponse>('/admin/themes')
  return data
}

export async function uploadTheme(file: File): Promise<ThemePackage> {
  const form = new FormData()
  form.append('theme', file)
  const { data } = await apiClient.post<ThemePackage>('/admin/themes/upload', form, {
    // The shared client defaults to JSON. Let the browser supply the multipart boundary.
    headers: { 'Content-Type': undefined },
    timeout: 120000
  })
  return data
}

export async function setActiveTheme(themeID: string | null): Promise<ActiveThemeResponse> {
  const { data } = await apiClient.put<ActiveThemeResponse>('/admin/themes/active', {
    theme_id: themeID
  })
  return data
}
