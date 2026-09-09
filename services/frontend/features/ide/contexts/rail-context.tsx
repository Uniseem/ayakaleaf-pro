'use client'

/**
 * The rail's state, from ide-react/context/rail-context.tsx.
 *
 * Which tab is selected and whether its panel is open are two things, not
 * one: closing the panel keeps the tab, so opening it again shows what was
 * there. Both are remembered per project. The panel itself is a resizable
 * panel, collapsed and expanded through its handle so that the saved size
 * survives a close.
 */

import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from 'react'
import type { ImperativePanelHandle } from 'react-resizable-panels'
import { usePersistedState, useEventListener } from '@/lib/hooks'
import { isMac } from '@/lib/os'
import { useProject } from './project-context'

export type RailTabKey =
  | 'file-tree'
  | 'integrations'
  | 'review-panel'
  | 'chat'
  | 'full-project-search'
  | 'dimensions'
  | 'workbench'

export type RailModalKey = 'keyboard-shortcuts' | 'contact-us' | 'dictionary' | 'labs'

export function dispatchOpenRailModal(key: RailModalKey) {
  window.dispatchEvent(new CustomEvent('ui:open-rail-modal', { detail: key }))
}

type RailContextValue = {
  selectedTab: RailTabKey
  isOpen: boolean
  setIsOpen: Dispatch<SetStateAction<boolean>>
  panelRef: React.RefObject<ImperativePanelHandle | null>
  togglePane: () => void
  handlePaneExpand: () => void
  handlePaneCollapse: () => void
  resizing: boolean
  setResizing: Dispatch<SetStateAction<boolean>>
  activeModal: RailModalKey | null
  setActiveModal: Dispatch<SetStateAction<RailModalKey | null>>
  openTab: (tab: RailTabKey) => void
  selectTab: (tab: RailTabKey) => void
}

const RailContext = createContext<RailContextValue | undefined>(undefined)

export function RailProvider({ children }: { children: ReactNode }) {
  const { projectId } = useProject()
  const [isOpen, setIsOpen] = usePersistedState(`rail-is-open-${projectId}`, true)
  const [resizing, setResizing] = useState(false)
  const [activeModal, setActiveModal] = useState<RailModalKey | null>(null)
  const panelRef = useRef<ImperativePanelHandle | null>(null)

  const togglePane = useCallback(() => {
    setIsOpen(value => !value)
  }, [setIsOpen])

  const handlePaneExpand = useCallback(() => {
    setIsOpen(true)
  }, [setIsOpen])

  const handlePaneCollapse = useCallback(() => {
    setIsOpen(false)
  }, [setIsOpen])

  const [selectedTab, setSelectedTab] = usePersistedState<RailTabKey>(`selected-rail-tab-${projectId}`, 'file-tree')

  // Keep the panel's collapsed state in step with isOpen and the tab.
  useLayoutEffect(() => {
    const panelHandle = panelRef.current
    if (panelHandle) {
      if (isOpen) {
        panelHandle.expand()
      } else {
        panelHandle.collapse()
      }
    }
  }, [isOpen, selectedTab])

  const selectTab = useCallback(
    (tab: RailTabKey) => {
      setSelectedTab(tab)
    },
    [setSelectedTab]
  )

  const openTab = useCallback(
    (tab: RailTabKey) => {
      setSelectedTab(tab)
      setIsOpen(true)
    },
    [setIsOpen, setSelectedTab]
  )

  useEventListener(
    'ui.toggle-review-panel' as keyof WindowEventMap,
    useCallback(() => {
      if (isOpen && selectedTab === 'review-panel') {
        handlePaneCollapse()
      } else {
        openTab('review-panel')
      }
    }, [handlePaneCollapse, selectedTab, isOpen, openTab])
  )

  useEventListener(
    'ui:open-rail-modal' as keyof WindowEventMap,
    useCallback((event: Event) => {
      setActiveModal((event as CustomEvent<RailModalKey>).detail)
    }, [])
  )

  useEventListener(
    'keydown',
    useCallback(
      (event: KeyboardEvent) => {
        if ((isMac ? event.metaKey : event.ctrlKey) && event.shiftKey && event.code === 'KeyF') {
          event.preventDefault()
          openTab('full-project-search')
        }
      },
      [openTab]
    )
  )

  const value = useMemo(
    () => ({
      selectedTab,
      isOpen,
      setIsOpen,
      panelRef,
      togglePane,
      handlePaneExpand,
      handlePaneCollapse,
      resizing,
      setResizing,
      activeModal,
      setActiveModal,
      openTab,
      selectTab,
    }),
    [selectedTab, isOpen, setIsOpen, togglePane, handlePaneExpand, handlePaneCollapse, resizing, activeModal, openTab, selectTab]
  )

  return <RailContext.Provider value={value}>{children}</RailContext.Provider>
}

export function useRailContext() {
  const context = useContext(RailContext)
  if (!context) {
    throw new Error('useRailContext is only available inside RailProvider')
  }
  return context
}
