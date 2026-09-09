'use client'

/**
 * What this person has chosen about how the editor behaves.
 *
 * These are preferences, not project data: two people in the same project see
 * it in their own font at their own size with their own keybindings. They are
 * held here so that the editor, the left menu and the settings page all read
 * one copy, and written through to the API so they follow the person to
 * another machine -- with local storage as the immediate copy, because a
 * preference that waits for a round trip feels broken.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  type ReactNode,
} from 'react'
import { usePersistedState } from '@/lib/hooks'
import { saveUserSettings } from '@/lib/user-settings'

export type OverallTheme = 'light' | 'dark'
export type Keybindings = 'default' | 'vim' | 'emacs'
export type PdfViewer = 'pdfjs' | 'native'
export type EditorMode = 'code' | 'visual'

export type UserSettings = {
  /** Source or rich text. */
  mode: EditorMode
  overallTheme: OverallTheme
  editorTheme: string
  fontSize: number
  fontFamily: 'monaco' | 'lucida' | 'opendyslexicmono'
  lineHeight: 'compact' | 'normal' | 'wide'
  keybindings: Keybindings
  autoComplete: boolean
  autoPairDelimiters: boolean
  syntaxValidation: boolean
  spellCheckLanguage: string
  mathPreview: boolean
  /** Show the PDF with the built-in viewer or the browser's. */
  pdfViewer: PdfViewer
  /** Dim the PDF when the interface is dark. */
  darkModePdf: boolean
  /** Show the document outline beside the file tree. */
  showOutline: boolean
  breadcrumbs: boolean
  /** The row of open-file tabs above the editor. */
  editorTabs: boolean
  /** Whether a single click opens a file in a temporary tab. */
  previewTabs: boolean
  nonBlinkingCursor: boolean
}

export const defaultSettings: UserSettings = {
  mode: 'code',
  overallTheme: 'light',
  editorTheme: 'textmate',
  fontSize: 12,
  fontFamily: 'monaco',
  lineHeight: 'normal',
  keybindings: 'default',
  autoComplete: true,
  autoPairDelimiters: true,
  syntaxValidation: true,
  spellCheckLanguage: 'en',
  mathPreview: true,
  pdfViewer: 'pdfjs',
  darkModePdf: false,
  showOutline: true,
  breadcrumbs: true,
  editorTabs: true,
  previewTabs: true,
  nonBlinkingCursor: false,
}

export type SettingsValue = UserSettings & {
  set: <K extends keyof UserSettings>(key: K, value: UserSettings[K]) => void
  setMany: (values: Partial<UserSettings>) => void
  reset: () => void
}

const SettingsContext = createContext<SettingsValue | undefined>(undefined)

export function SettingsProvider({
  initial,
  children,
}: {
  initial?: Partial<UserSettings>
  children: ReactNode
}) {
  const [settings, setSettings] = usePersistedState<UserSettings>(
    'user.settings',
    { ...defaultSettings, ...initial }
  )

  const set = useCallback(
    <K extends keyof UserSettings>(key: K, value: UserSettings[K]) => {
      setSettings(previous => {
        const next = { ...previous, [key]: value }
        // Told to the server after the fact: the setting has already taken
        // effect locally, and failing to record it is not worth interrupting
        // somebody who is trying to change their font size.
        void saveUserSettings({ [key]: value }).catch(() => {})
        return next
      })
    },
    [setSettings]
  )

  const setMany = useCallback(
    (values: Partial<UserSettings>) => {
      setSettings(previous => ({ ...previous, ...values }))
      void saveUserSettings(values).catch(() => {})
    },
    [setSettings]
  )

  const reset = useCallback(() => {
    setSettings(defaultSettings)
    void saveUserSettings(defaultSettings).catch(() => {})
  }, [setSettings])

  // The interface theme is a document-level thing: the PDF viewer, the
  // dropdowns rendered in portals and the scrollbars are all outside this
  // tree, so it is set on the root element rather than passed down.
  useEffect(() => {
    document.documentElement.dataset.theme = settings.overallTheme
    document.documentElement.style.colorScheme = settings.overallTheme
  }, [settings.overallTheme])

  // Defaults underneath whatever was stored, so a setting added after the
  // stored copy was written still has a value.
  const value = useMemo<SettingsValue>(
    () => ({ ...defaultSettings, ...settings, fontFamily: legacyFontFamily(settings.fontFamily), set, setMany, reset }),
    [settings, set, setMany, reset]
  )

  return (
    <SettingsContext.Provider value={value}>
      {children}
    </SettingsContext.Provider>
  )
}

/** Names stored before the font families took the original's names. */
function legacyFontFamily(value: string | undefined): UserSettings['fontFamily'] {
  switch (value) {
    case 'monospace':
    case 'monaco':
    case undefined:
      return 'monaco'
    case 'opendyslexic':
    case 'opendyslexicmono':
      return 'opendyslexicmono'
    case 'lucida':
      return 'lucida'
    default:
      return 'monaco'
  }
}

export function useSettings(): SettingsValue {
  const value = useContext(SettingsContext)
  if (!value) {
    throw new Error('useSettings must be used inside a SettingsProvider')
  }
  return value
}

/** The CSS the editor needs for the chosen font and spacing. */
export function editorFontStyle(settings: UserSettings): React.CSSProperties {
  const families: Record<UserSettings['fontFamily'], string> = {
    monaco: "Monaco, Menlo, 'Ubuntu Mono', Consolas, monospace",
    lucida: "'Lucida Console', 'Source Code Pro', monospace",
    opendyslexicmono: "'OpenDyslexic Mono', monospace",
  }
  const heights: Record<UserSettings['lineHeight'], number> = {
    compact: 1.33,
    normal: 1.6,
    wide: 2,
  }
  return {
    fontFamily: families[settings.fontFamily],
    fontSize: `${settings.fontSize}px`,
    lineHeight: heights[settings.lineHeight],
  }
}
