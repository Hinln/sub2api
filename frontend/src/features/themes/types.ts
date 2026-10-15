export const THEME_SHADES = ['50', '100', '200', '300', '400', '500', '600', '700', '800', '900', '950'] as const
export const THEME_PALETTES = ['primary', 'gray', 'dark', 'accent'] as const

export type ThemeShade = typeof THEME_SHADES[number]
export type ThemePalette = typeof THEME_PALETTES[number]
export type ThemeFont = 'system' | 'serif' | 'mono'

export interface ThemeTokens {
  colors?: Partial<Record<ThemePalette, Partial<Record<ThemeShade, string>>>>
  font?: ThemeFont
  radius_scale?: number
}

export interface ThemeManifest {
  schema_version: 1
  id: string
  name: string
  version: string
  description?: string
  author?: string
  tokens: ThemeTokens
}

export interface ThemePackage {
  id: string
  manifest: ThemeManifest
  installed_at: string
}

export interface ActiveThemeResponse {
  active_theme: ThemePackage | null
}

export interface ThemeListResponse extends ActiveThemeResponse {
  themes: ThemePackage[]
}
