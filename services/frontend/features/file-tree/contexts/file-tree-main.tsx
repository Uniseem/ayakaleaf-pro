'use client'

/**
 * Where the context menu is, from file-tree/contexts/file-tree-main.
 *
 * Held here rather than in the row that opened it because the menu is drawn
 * into the body at page coordinates: the row that was right-clicked is not
 * the element the menu belongs to, and a menu inside a scrolling list would
 * scroll away from the pointer that opened it.
 */

import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'

export type ContextMenuCoords = { top: number; left: number }

const FileTreeMainContext = createContext<
  | {
      contextMenuCoords: ContextMenuCoords | null
      setContextMenuCoords: (value: ContextMenuCoords | null) => void
    }
  | undefined
>(undefined)

export function useFileTreeMainContext() {
  const context = useContext(FileTreeMainContext)
  if (!context) {
    throw new Error('useFileTreeMainContext is only available inside FileTreeMainProvider')
  }
  return context
}

export function FileTreeMainProvider({ children }: { children: ReactNode }) {
  const [contextMenuCoords, setContextMenuCoords] = useState<ContextMenuCoords | null>(null)

  const value = useMemo(
    () => ({ contextMenuCoords, setContextMenuCoords }),
    [contextMenuCoords]
  )

  return <FileTreeMainContext.Provider value={value}>{children}</FileTreeMainContext.Provider>
}
