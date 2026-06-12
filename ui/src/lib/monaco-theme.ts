import { formatHex } from 'culori'
import type { Monaco } from '@monaco-editor/react'

export function getCssVariableColor(variable: string, fallback: string) {
  if (typeof window === 'undefined') {
    return fallback
  }
  const raw = getComputedStyle(document.documentElement)
    .getPropertyValue(variable)
    .trim()
  if (!raw) {
    return fallback
  }
  return formatHex(raw) || fallback
}

export function defineMonacoLogThemes(monaco: Monaco) {
  monaco.editor.defineTheme('log-theme-classic', {
    base: 'vs-dark',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': '#0f172a',
      'editor.foreground': '#e2e8f0',
    },
  })
  monaco.editor.defineTheme('log-theme-light', {
    base: 'vs',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': '#ffffff',
      'editor.foreground': '#0f172a',
    },
  })
  monaco.editor.defineTheme('log-theme-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': '#18181b',
      'editor.foreground': '#f4f4f5',
    },
  })
}

export function defineMonacoBackgroundThemes(
  monaco: Monaco,
  {
    darkThemeName,
    lightThemeName,
    backgroundColor,
  }: {
    darkThemeName: string
    lightThemeName: string
    backgroundColor: string
  }
) {
  monaco.editor.defineTheme(darkThemeName, {
    base: 'vs-dark',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': backgroundColor,
    },
  })
  monaco.editor.defineTheme(lightThemeName, {
    base: 'vs',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': backgroundColor,
    },
  })
}

export function useMonacoBackgroundColor(
  variable: string,
  themeMode: 'dark' | 'light',
  _colorTheme: string
) {
  return getCssVariableColor(variable, themeMode === 'dark' ? '#18181b' : '#fff')
}
