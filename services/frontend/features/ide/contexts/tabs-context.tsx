'use client'

/**
 * The open-file tabs, from ide-react/context/tabs-context.tsx.
 *
 * A tab is a file id and a lifetime: a temporary tab is the one a single
 * click opened, shown in italics and replaced by the next single click, and
 * it becomes permanent on a double click or an edit. Two files with the same
 * name show as much of their paths as it takes to tell them apart. The list
 * is remembered per project.
 */

import React, { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { usePersistedState } from '@/lib/hooks'
import { tabsEvents, TAB_USER_EDIT_EVENT } from '@/features/source-editor/extensions/tabs-listener'
import type { FileEntry } from '@/lib/editor'
import { useProject } from './project-context'
import { useEditor } from './editor-context'
import { useSettings } from './settings-context'

export const TAB_TRANSFER_TYPE = 'application/x-overleaf-tab-id'

type Lifetime = 'temporary' | 'permanent'

type PersistedTabInfo = { id: string; lifetime: Lifetime }

export type EditorFileTab = {
  id: string
  name: string
  displayPath: string
  isLinkedFile: boolean
  lifetime: Lifetime
}

export type TabsContextMenuTarget = {
  top: number
  left: number
  tabId: string
}

type TabsContextValue = {
  tabs: EditorFileTab[]
  openTab: (id: string) => void
  closeTab: (id: string) => void
  closeOtherTabs: (id: string) => void
  makeTabPermanent: (id: string) => void
  closeToRight: (id: string) => void
  moveTab: (sourceTabId: string, targetTabId: string, position: 'left' | 'right') => void
  contextMenuTarget: TabsContextMenuTarget | null
  setContextMenuTarget: React.Dispatch<React.SetStateAction<TabsContextMenuTarget | null>>
  headerSlot: HTMLElement | null
  setHeaderSlot: React.Dispatch<React.SetStateAction<HTMLElement | null>>
}

const TabsContext = createContext<TabsContextValue | undefined>(undefined)

export function useAreTabsEnabled() {
  const { editorTabs } = useSettings()
  return Boolean(editorTabs)
}

/** As much of each path as it takes for the names to differ. */
function disambiguatePaths(entries: FileEntry[]): Map<string, string> {
  const results = new Map<string, string>()
  const tabs = entries.map(entry => ({
    id: entry.id,
    name: entry.name,
    segments: entry.path.split('/').slice(0, -1),
    depth: 0,
  }))

  const displayPath = (tab: (typeof tabs)[number]) =>
    tab.segments
      .slice(Math.max(tab.segments.length - tab.depth, 0))
      .concat(tab.name)
      .join('/')

  for (;;) {
    const collisions = new Map<string, typeof tabs>()
    for (const tab of tabs) {
      if (results.has(tab.id)) {
        continue
      }
      const path = displayPath(tab)
      const list = collisions.get(path) ?? []
      list.push(tab)
      collisions.set(path, list)
    }

    let hasCollisions = false
    for (const [path, colliding] of collisions) {
      if (colliding.length === 1) {
        results.set(colliding[0]!.id, path)
      } else {
        for (const tab of colliding) {
          if (tab.depth < tab.segments.length) {
            tab.depth++
            hasCollisions = true
          } else {
            results.set(tab.id, tab.name)
          }
        }
      }
    }
    if (!hasCollisions) {
      break
    }
  }
  return results
}

export function TabsProvider({ children }: { children: ReactNode }) {
  const { projectId, files, entryById } = useProject()
  const editor = useEditor()
  const tabsEnabled = useAreTabsEnabled()
  const { previewTabs } = useSettings()

  const [openTabs, setOpenTabs] = usePersistedState<PersistedTabInfo[]>(`open-tabs:${projectId}`, [])
  const [contextMenuTarget, setContextMenuTarget] = useState<TabsContextMenuTarget | null>(null)
  const [headerSlot, setHeaderSlot] = useState<HTMLElement | null>(null)

  const openEntityId = editor.currentFile?.id ?? editor.current?.id ?? null

  const tabs = useMemo(() => {
    if (!tabsEnabled) {
      return []
    }
    const found = openTabs
      .map(tab => ({ lifetime: tab.lifetime, entry: entryById(tab.id) }))
      .filter((tab): tab is { lifetime: Lifetime; entry: FileEntry } => Boolean(tab.entry))

    const pathLookup = disambiguatePaths(found.map(tab => tab.entry))

    return found.map(tab => ({
      id: tab.entry.id,
      name: tab.entry.name,
      displayPath: pathLookup.get(tab.entry.id) || tab.entry.name,
      isLinkedFile: false,
      lifetime: tab.lifetime,
    }))
  }, [openTabs, tabsEnabled, entryById])

  const openTab = useCallback(
    (id: string) => {
      const entry = entryById(id)
      if (entry && entry.kind !== 'folder') {
        editor.open(entry)
      }
    },
    [entryById, editor]
  )

  const closeTab = useCallback(
    (id: string) => {
      if (openTabs.length <= 1) {
        return
      }
      if (id === openEntityId) {
        const currentIndex = openTabs.findIndex(tab => tab.id === id)
        if (currentIndex === -1) {
          return
        }
        const nextTab = openTabs[currentIndex + 1] || openTabs[currentIndex - 1]
        if (!nextTab) {
          return
        }
        openTab(nextTab.id)
      }
      setOpenTabs(current => current.filter(tab => tab.id !== id))
    },
    [openTabs, openEntityId, setOpenTabs, openTab]
  )

  const closeOtherTabs = useCallback(
    (id: string) => {
      if (id !== openEntityId) {
        openTab(id)
      }
      setOpenTabs(current => current.filter(tab => tab.id === id))
    },
    [openEntityId, openTab, setOpenTabs]
  )

  const closeToRight = useCallback(
    (id: string) => {
      const tabIndex = openTabs.findIndex(tab => tab.id === id)
      if (tabIndex === -1) {
        return
      }
      const openTabIndex = openTabs.findIndex(tab => tab.id === openEntityId)
      if (openTabIndex === -1 || openTabIndex > tabIndex) {
        openTab(id)
      }
      setOpenTabs(current => {
        const currentIndex = current.findIndex(tab => tab.id === id)
        if (currentIndex === -1) {
          return current
        }
        return current.slice(0, currentIndex + 1)
      })
    },
    [setOpenTabs, openTabs, openEntityId, openTab]
  )

  const moveTab = useCallback(
    (sourceTabId: string, targetTabId: string, position: 'left' | 'right') => {
      if (sourceTabId === targetTabId) {
        return
      }
      setOpenTabs(current => {
        const sourceTabIndex = current.findIndex(tab => tab.id === sourceTabId)
        const targetTabIndex = current.findIndex(tab => tab.id === targetTabId)
        if (sourceTabIndex === -1 || targetTabIndex === -1) {
          return current
        }
        if (
          (position === 'right' && targetTabIndex === sourceTabIndex - 1) ||
          (position === 'left' && targetTabIndex === sourceTabIndex + 1)
        ) {
          return current
        }
        return arrayMove(current, sourceTabIndex, targetTabIndex, position)
      })
    },
    [setOpenTabs]
  )

  const makeTabPermanent = useCallback(
    (id: string) => {
      setOpenTabs(current => current.map(tab => (tab.id === id ? { ...tab, lifetime: 'permanent' } : tab)))
    },
    [setOpenTabs]
  )

  // An edit makes the tab permanent, the way a double click does.
  useEffect(() => {
    if (!tabsEnabled || !openEntityId) {
      return
    }
    const listener = () => makeTabPermanent(openEntityId)
    tabsEvents.addEventListener(TAB_USER_EDIT_EVENT, listener)
    return () => tabsEvents.removeEventListener(TAB_USER_EDIT_EVENT, listener)
  }, [tabsEnabled, openEntityId, makeTabPermanent])

  // Whatever is open has a tab; a single click's tab replaces the last one.
  useEffect(() => {
    if (!tabsEnabled || !openEntityId) {
      return
    }
    setOpenTabs(current => {
      if (current.find(tab => tab.id === openEntityId)) {
        return current
      }
      return [...current.filter(tab => tab.lifetime !== 'temporary'), { id: openEntityId, lifetime: previewTabs ? 'temporary' : 'permanent' }]
    })
  }, [openEntityId, previewTabs, setOpenTabs, tabsEnabled])

  // A tab for a file that has been deleted goes away with it.
  useEffect(() => {
    if (!tabsEnabled || files.length === 0) {
      return
    }
    setOpenTabs(current => {
      const pruned = current.filter(tab => Boolean(entryById(tab.id)))
      return pruned.length === current.length ? current : pruned
    })
  }, [files, entryById, setOpenTabs, tabsEnabled])

  const value = useMemo(
    () => ({
      tabs,
      openTab,
      closeTab,
      closeOtherTabs,
      moveTab,
      makeTabPermanent,
      contextMenuTarget,
      setContextMenuTarget,
      closeToRight,
      headerSlot,
      setHeaderSlot,
    }),
    [tabs, openTab, closeTab, closeOtherTabs, moveTab, makeTabPermanent, contextMenuTarget, closeToRight, headerSlot]
  )

  return <TabsContext.Provider value={value}>{children}</TabsContext.Provider>
}

export function useTabsContext() {
  const value = useContext(TabsContext)
  if (!value) {
    throw new Error('useTabsContext can only be used inside TabsProvider')
  }
  return value
}

function arrayMove<T>(array: T[], sourceIndex: number, targetIndex: number, side: 'left' | 'right'): T[] {
  const result = [...array]
  const [movedItem] = result.splice(sourceIndex, 1)
  let newIndex = targetIndex
  if (sourceIndex < targetIndex) {
    newIndex -= 1
  }
  if (side === 'right') {
    newIndex += 1
  }
  result.splice(newIndex, 0, movedItem as T)
  return result
}
