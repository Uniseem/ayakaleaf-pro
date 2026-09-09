'use client'

/**
 * The document outline: the headings the editor found, which one the cursor
 * is in, and whether the outline pane is open.
 *
 * The headings come from the editor's syntax tree rather than from a scan of
 * the text, so they are the same headings the editor folds on, and they are
 * pushed here by the editor as the projection changes.
 */

import { createContext, useCallback, useContext, useMemo, useState, type Dispatch, type ReactNode, type SetStateAction } from 'react'
import { usePersistedState, useEventListener } from '@/lib/hooks'
import { useProject } from './project-context'
import { useEditor } from './editor-context'
import { isValidTeXFile } from '@/features/source-editor/utils/misc'

export type PartialFlatOutline = {
  level: number
  title: string
  line: number
}[]

export type FlatOutlineState =
  | {
      items: PartialFlatOutline
      partial: boolean
    }
  | undefined

export type OutlineValue = {
  flatOutline: FlatOutlineState
  setFlatOutline: Dispatch<SetStateAction<FlatOutlineState>>
  highlightedLine: number
  jumpToLine: (lineNumber: number, syncToPdf: boolean) => void
  canShowOutline: boolean
  outlineExpanded: boolean
  toggleOutlineExpanded: () => void
  expandOutline: () => void
  collapseOutline: () => void
}

const OutlineContext = createContext<OutlineValue | undefined>(undefined)

export function OutlineProvider({ children }: { children: ReactNode }) {
  const [flatOutline, setFlatOutline] = useState<FlatOutlineState>(undefined)
  const [currentlyHighlightedLine, setCurrentlyHighlightedLine] = useState<number>(-1)
  const [ignoreNextScroll, setIgnoreNextScroll] = useState<boolean>(false)

  useEventListener(
    'scroll:editor:update' as keyof WindowEventMap,
    useCallback(
      (event: Event) => {
        if (ignoreNextScroll) {
          setIgnoreNextScroll(false)
          return
        }
        setCurrentlyHighlightedLine(Number((event as CustomEvent<number>).detail) + 1)
      },
      [ignoreNextScroll]
    )
  )

  useEventListener(
    'cursor:editor:update' as keyof WindowEventMap,
    useCallback((event: Event) => {
      setCurrentlyHighlightedLine((event as CustomEvent<{ row: number }>).detail.row + 1)
    }, [])
  )

  const jumpToLine = useCallback((lineNumber: number, syncToPdf: boolean) => {
    setIgnoreNextScroll(true)
    window.dispatchEvent(
      new CustomEvent('ide:goto-line', {
        detail: { line: lineNumber - 1, column: 0, syncToPdf },
      })
    )
  }, [])

  const highlightedLine = useMemo(
    () => closestSectionLineNumber(flatOutline?.items, currentlyHighlightedLine),
    [flatOutline, currentlyHighlightedLine]
  )

  const { current, currentFile } = useEditor()
  const openDocName = current?.name
  const isTexFile = useMemo(() => (openDocName ? isValidTeXFile(openDocName) : false), [openDocName])

  const { projectId } = useProject()
  const [outlineExpanded, setOutlineExpanded] = usePersistedState(`file_outline.expanded.${projectId}`, true)

  const canShowOutline = Boolean(current) && !currentFile && isTexFile

  const expandOutline = useCallback(() => {
    if (canShowOutline) {
      setOutlineExpanded(true)
    }
  }, [canShowOutline, setOutlineExpanded])

  const collapseOutline = useCallback(() => {
    if (canShowOutline) {
      setOutlineExpanded(false)
    }
  }, [canShowOutline, setOutlineExpanded])

  const toggleOutlineExpanded = useCallback(() => {
    if (canShowOutline) {
      setOutlineExpanded(value => !value)
    }
  }, [canShowOutline, setOutlineExpanded])

  const value = useMemo<OutlineValue>(
    () => ({
      flatOutline,
      setFlatOutline,
      highlightedLine,
      jumpToLine,
      canShowOutline,
      outlineExpanded,
      toggleOutlineExpanded,
      expandOutline,
      collapseOutline,
    }),
    [flatOutline, highlightedLine, jumpToLine, canShowOutline, outlineExpanded, toggleOutlineExpanded, expandOutline, collapseOutline]
  )

  return <OutlineContext.Provider value={value}>{children}</OutlineContext.Provider>
}

export function useOutlineContext(): OutlineValue {
  const context = useContext(OutlineContext)
  if (!context) {
    throw new Error('useOutlineContext is only available inside OutlineProvider')
  }
  return context
}

/** The heading the cursor is inside: the last one at or above the line. */
function closestSectionLineNumber(outline: PartialFlatOutline | undefined, lineNumber: number): number {
  if (!outline) {
    return -1
  }
  let highestLine = -1
  for (const section of outline) {
    if (section.line > lineNumber) {
      return highestLine
    }
    highestLine = section.line
  }
  return highestLine
}
