'use client'

/**
 * Where everything is on screen.
 *
 * The editor is several panels that can each be open, closed, or in front of
 * another, and almost every one of them has a control somewhere else that
 * opens it: the toolbar opens the left menu, a file in the tree replaces what
 * the editor is showing, a compile error opens the log. Holding that in one
 * place is what stops those controls from having to know about each other.
 *
 * What is remembered across reloads is remembered deliberately: the split
 * between editor and PDF, and whether the review panel and chat were open,
 * because coming back to a layout you did not choose is worse than the one
 * frame it costs to correct it.
 */

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { usePersistedState } from '@/lib/hooks'

/** Side by side, or one pane at a time. */
export type IdeLayout = 'sideBySide' | 'flat'

/** What the main pane is showing. */
export type IdeView = 'editor' | 'file' | 'pdf' | 'history'

/** Which panel the rail is showing, or none. */
export type RailTab =
  | 'file-tree'
  | 'full-project-search'
  | 'integrations'
  | 'review-panel'
  | 'chat'
  | 'help'

export type LayoutValue = {
  view: IdeView
  setView: (view: IdeView) => void
  /** Goes back to whatever was showing before history took over. */
  restoreView: () => void

  pdfLayout: IdeLayout
  changeLayout: (layout: IdeLayout, view?: IdeView) => void
  /** The same as changeLayout; the original's name for the menu's handler. */
  handleChangeLayout: (layout: IdeLayout, view?: IdeView) => void
  /** Opens the PDF in its own tab. Not offered yet, so undefined. */
  handleDetach?: () => void
  detachRole: 'detacher' | 'detached' | null
  detachIsLinked: boolean
  /** Whether the PDF is on screen, in this window or another. */
  pdfPreviewOpen: boolean
  /** Whether the editor pane should be on screen at all. */
  editorIsOpen: boolean
  /** Whether the PDF pane should be on screen at all. */
  pdfIsOpen: boolean

  railTab: RailTab | null
  setRailTab: (tab: RailTab | null) => void
  toggleRailTab: (tab: RailTab) => void
  railIsOpen: boolean

  leftMenuShown: boolean
  setLeftMenuShown: (shown: boolean) => void

  chatIsOpen: boolean
  setChatIsOpen: (open: boolean) => void
  reviewPanelOpen: boolean
  setReviewPanelOpen: (open: boolean) => void

  showLogs: boolean
  setShowLogs: (show: boolean) => void
  toggleLogs: () => void

  focusMode: boolean
  setFocusMode: (on: boolean) => void
}

const LayoutContext = createContext<LayoutValue | undefined>(undefined)

export function LayoutProvider({ children }: { children: ReactNode }) {
  const [view, setViewState] = useState<IdeView>('editor')
  // What to go back to when history closes. History is entered from anywhere
  // and left with one button, so the button needs somewhere to return to.
  const [viewBeforeHistory, setViewBeforeHistory] = useState<IdeView>('editor')

  const [pdfLayout, setPdfLayout] = usePersistedState<IdeLayout>(
    'ide.pdfLayout',
    'sideBySide'
  )
  const [railTab, setRailTabState] = usePersistedState<RailTab | null>(
    'ide.railTab',
    'file-tree'
  )
  const [leftMenuShown, setLeftMenuShown] = useState(false)
  const [chatIsOpen, setChatIsOpen] = usePersistedState('ide.chatOpen', false)
  const [reviewPanelOpen, setReviewPanelOpen] = usePersistedState(
    'ide.reviewPanelOpen',
    false
  )
  const [showLogs, setShowLogs] = useState(false)
  const [focusMode, setFocusMode] = usePersistedState('ide.focusMode', false)

  const setView = useCallback((next: IdeView) => {
    setViewState(previous => {
      if (previous !== 'history') {
        setViewBeforeHistory(previous)
      }
      return next
    })
  }, [])

  const restoreView = useCallback(() => {
    setViewState(viewBeforeHistory)
  }, [viewBeforeHistory])

  const changeLayout = useCallback(
    (layout: IdeLayout, next?: IdeView) => {
      setPdfLayout(layout)
      if (next) {
        setView(next)
      } else if (layout === 'sideBySide') {
        setView('editor')
      }
    },
    [setPdfLayout, setView]
  )

  const setRailTab = useCallback(
    (tab: RailTab | null) => setRailTabState(tab),
    [setRailTabState]
  )

  const toggleRailTab = useCallback(
    (tab: RailTab) => setRailTabState(current => (current === tab ? null : tab)),
    [setRailTabState]
  )

  const toggleLogs = useCallback(() => setShowLogs(current => !current), [])

  const value = useMemo<LayoutValue>(() => {
    // Side by side shows both; flat shows whichever the view names. Focus mode
    // hides the rail but not the panes, so it is not part of this.
    const editorIsOpen =
      view === 'editor' || view === 'file' || pdfLayout === 'sideBySide'
    const pdfIsOpen = view === 'pdf' || pdfLayout === 'sideBySide'

    return {
      view,
      setView,
      restoreView,
      pdfLayout,
      changeLayout,
      handleChangeLayout: changeLayout,
      handleDetach: undefined,
      detachRole: null,
      detachIsLinked: false,
      pdfPreviewOpen: pdfIsOpen && view !== 'history',
      editorIsOpen: editorIsOpen && view !== 'history',
      pdfIsOpen: pdfIsOpen && view !== 'history',
      railTab: focusMode ? null : railTab,
      setRailTab,
      toggleRailTab,
      railIsOpen: !focusMode && railTab !== null,
      leftMenuShown,
      setLeftMenuShown,
      chatIsOpen,
      setChatIsOpen,
      reviewPanelOpen,
      setReviewPanelOpen,
      showLogs,
      setShowLogs,
      toggleLogs,
      focusMode,
      setFocusMode,
    }
  }, [
    view,
    setView,
    restoreView,
    pdfLayout,
    changeLayout,
    railTab,
    setRailTab,
    toggleRailTab,
    leftMenuShown,
    setLeftMenuShown,
    chatIsOpen,
    setChatIsOpen,
    reviewPanelOpen,
    setReviewPanelOpen,
    showLogs,
    toggleLogs,
    focusMode,
    setFocusMode,
  ])

  return <LayoutContext.Provider value={value}>{children}</LayoutContext.Provider>
}

export function useLayout(): LayoutValue {
  const value = useContext(LayoutContext)
  if (!value) {
    throw new Error('useLayout must be used inside a LayoutProvider')
  }
  return value
}
