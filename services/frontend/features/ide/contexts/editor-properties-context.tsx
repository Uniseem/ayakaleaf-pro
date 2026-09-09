'use client'

/**
 * How the editor is being shown right now: source or visual, whether the
 * symbol palette is open, whether edits are tracked.
 *
 * Visual mode is a setting (it follows the person between projects); the
 * symbol palette is not (it closes with the page). Both are read from here
 * so the toolbar, the menu bar and the editor agree.
 */

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { useSettings } from './settings-context'
import { useReview } from './review-context'

export type EditorPropertiesValue = {
  showVisual: boolean
  setShowVisual: (visual: boolean) => void
  showSymbolPalette: boolean
  toggleSymbolPalette: () => void
  setShowSymbolPalette: (show: boolean) => void
  trackChanges: boolean
}

const EditorPropertiesContext = createContext<EditorPropertiesValue | undefined>(undefined)

export function EditorPropertiesProvider({ children }: { children: ReactNode }) {
  const settings = useSettings()
  const review = useReview()
  const [showSymbolPalette, setShowSymbolPalette] = useState(false)

  const setShowVisual = useCallback(
    (visual: boolean) => {
      settings.set('mode', visual ? 'visual' : 'code')
    },
    [settings]
  )

  const toggleSymbolPalette = useCallback(() => {
    setShowSymbolPalette(value => !value)
  }, [])

  const value = useMemo<EditorPropertiesValue>(
    () => ({
      showVisual: settings.mode === 'visual',
      setShowVisual,
      showSymbolPalette,
      toggleSymbolPalette,
      setShowSymbolPalette,
      trackChanges: review.mode === 'suggesting',
    }),
    [settings.mode, setShowVisual, showSymbolPalette, toggleSymbolPalette, review.mode]
  )

  return <EditorPropertiesContext.Provider value={value}>{children}</EditorPropertiesContext.Provider>
}

export function useEditorPropertiesContext(): EditorPropertiesValue {
  const context = useContext(EditorPropertiesContext)
  if (!context) {
    throw new Error('useEditorPropertiesContext is only available inside EditorPropertiesProvider')
  }
  return context
}
