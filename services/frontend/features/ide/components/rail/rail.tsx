'use client'

/**
 * The rail down the left, from ide-react/components/rail/rail.tsx.
 *
 * A strip of tabs, one panel showing the selected tab, and a handle between
 * the panel and the editor. Clicking the open tab closes the panel; the
 * help and settings buttons sit at the foot; tabs that do not fit go into
 * an overflow menu.
 */

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Panel } from 'react-resizable-panels'
import { useTranslation } from '@/lib/i18n'
import { useEventListener, usePrevious, useResizeObserver } from '@/lib/hooks'
import { isMac } from '@/lib/os'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useRailContext, type RailTabKey } from '@/features/ide/contexts/rail-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useSite } from '@/features/ide/contexts/site-context'
import { useCommandProvider } from '@/features/ide/contexts/command-registry-context'
import { HorizontalResizeHandle, HorizontalToggler } from '@/features/ide/components/resize/resize-handles'
import { FileTreeOutlinePanel } from '@/features/file-tree/file-tree-outline-panel'
import { ProjectSearchPanel } from '@/features/ide/components/project-search'
import { ChatPane, ChatIndicator } from '@/features/chat/chat-pane'
import { IntegrationsPanel } from '@/features/integrations-panel/integrations-panel'
import HotkeysModal from '@/features/hotkeys-modal/hotkeys-modal'
import { RailActionElement, RailHelpDropdown, RailOverflowDropdown, RailTab, shouldIncludeElement, type RailAction, type RailElement } from './rail-parts'

export function RailLayout() {
  const { t } = useTranslation()
  const { selectedTab, openTab, isOpen, setIsOpen, togglePane, selectTab } = useRailContext()
  const { features, isRestrictedTokenMember } = useProject()
  const site = useSite()
  const { view, setLeftMenuShown, focusMode } = useLayout()

  const isHistoryView = view === 'history'

  useEventListener(
    'ui:select-rail-tab' as keyof WindowEventMap,
    useCallback(
      (event: Event) => {
        const {
          detail: { tab, open },
        } = event as CustomEvent<{ tab: RailTabKey; open: boolean }>
        selectTab(tab)
        setIsOpen(open)
      },
      [selectTab, setIsOpen]
    )
  )

  const railTabs: RailElement[] = useMemo(
    () => [
      {
        key: 'file-tree',
        icon: 'description',
        title: t('file_tree'),
        component: <FileTreeOutlinePanel />,
        mountOnFirstLoad: true,
      },
      {
        key: 'full-project-search',
        icon: 'search',
        title: t('project_search'),
        component: <ProjectSearchPanel />,
      },
      {
        key: 'integrations',
        icon: 'integration_instructions',
        title: t('integrations'),
        component: <IntegrationsPanel />,
        hide: !site.gitBridgeEnabled,
      },
      {
        key: 'review-panel',
        icon: 'rate_review',
        title: t('review_panel'),
        component: null,
        hide: !features.trackChangesVisible,
        disabled: view !== 'editor',
      },
      {
        key: 'chat',
        icon: 'forum',
        component: <ChatPane />,
        indicator: <ChatIndicator />,
        title: t('chat'),
        hide: !site.capabilities.includes('chat') || isRestrictedTokenMember,
      },
    ],
    [t, features.trackChangesVisible, view, isRestrictedTokenMember, site.gitBridgeEnabled, site.capabilities]
  )

  const railActions: RailAction[] = useMemo(
    () => [
      {
        key: 'support',
        icon: 'help',
        title: t('help'),
        dropdown: <RailHelpDropdown />,
      },
      {
        key: 'settings',
        icon: 'settings',
        title: t('settings'),
        action: () => setLeftMenuShown(true),
      },
    ],
    [setLeftMenuShown, t]
  )

  useCommandProvider(
    () => [
      {
        id: 'open-settings',
        handler: () => setLeftMenuShown(true),
        menuLabel: t('settings'),
        label: t('open_settings'),
      },
    ],
    [t, setLeftMenuShown]
  )

  const onTabSelect = useCallback(
    (key: string | null) => {
      if (key === selectedTab) {
        togglePane()
      } else {
        if (!railTabs.some(tab => shouldIncludeElement(tab) && tab.key === key)) {
          return
        }
        const keyOrDefault = (key ?? 'file-tree') as RailTabKey
        openTab(keyOrDefault)
      }
    },
    [openTab, togglePane, selectedTab, railTabs]
  )

  useEffect(() => {
    const validTabKeys = railTabs.filter(shouldIncludeElement).map(tab => tab.key)
    if (!validTabKeys.includes(selectedTab) && isOpen) {
      openTab('file-tree')
    }
  }, [railTabs, selectedTab, openTab, isOpen])

  // The review panel takes the editor's own width rather than the rail's, so
  // while it is open the rail shows no panel and no handle of its own.
  const isReviewPanelOpen = selectedTab === 'review-panel' && isOpen && !isHistoryView

  const { tabsInRail, tabsInOverflow, tabWrapperRef } = useRailOverflow(railTabs)

  const moreOptionsAction: RailAction = useMemo(
    () => ({
      key: 'more-options',
      icon: 'more_vert',
      title: t('more_options'),
      hide: tabsInOverflow.length === 0,
      dropdown: <RailOverflowDropdown tabs={tabsInOverflow} isOpen={isOpen} selectedTab={selectedTab} onSelect={onTabSelect} />,
    }),
    [t, isOpen, selectedTab, tabsInOverflow, onTabSelect]
  )

  return (
    <>
      <nav className={['ide-rail', isHistoryView || focusMode ? 'hidden' : ''].filter(Boolean).join(' ')} aria-label={t('sidebar')}>
        <div className="nav ide-rail-tabs-nav" role="tablist">
          <div className="ide-rail-tabs-wrapper" ref={tabWrapperRef}>
            {tabsInRail.filter(shouldIncludeElement).map(({ icon, key, indicator, title, disabled, ref }) => (
              <RailTab
                open={isOpen && selectedTab === key}
                key={key}
                eventKey={key}
                icon={icon}
                indicator={indicator}
                title={title}
                disabled={disabled}
                ref={ref}
                onSelect={onTabSelect}
              />
            ))}
            <RailActionElement key="more-options" action={moreOptionsAction} />
          </div>
          <nav aria-label={t('help_editor_settings')}>
            {railActions.map(action => (
              <RailActionElement key={action.key} action={action} />
            ))}
          </nav>
        </div>
      </nav>
      <RailPanel isReviewPanelOpen={isReviewPanelOpen} isHistoryView={isHistoryView} railTabs={railTabs} focusMode={focusMode} />
      <RailResizeHandle isReviewPanelOpen={isReviewPanelOpen} focusMode={focusMode} />
      <RailModals />
    </>
  )
}

function useRailOverflow(railTabs: RailElement[]) {
  const [tabsInRail, setTabsInRail] = useState<RailElement[]>(railTabs)
  const [tabsInOverflow, setTabsInOverflow] = useState<RailElement[]>([])
  const tabWrapperRef = useRef<HTMLDivElement | null>(null)
  const size = useResizeObserver(tabWrapperRef)

  useEffect(() => {
    const element = tabWrapperRef.current
    if (!element) {
      return
    }
    const height = element.offsetHeight
    const tabHeight = (element.querySelector('.ide-rail-tab-link')?.clientHeight ?? 32) + 4
    const numTabsToFit = Math.floor(height / tabHeight)

    if (numTabsToFit >= railTabs.length) {
      setTabsInRail(railTabs)
      setTabsInOverflow([])
    } else {
      const sliceIndex = Math.max(numTabsToFit - 1, 0)
      setTabsInRail(railTabs.slice(0, sliceIndex))
      setTabsInOverflow(railTabs.slice(sliceIndex))
    }
  }, [railTabs, size.height])

  return { tabsInRail, tabsInOverflow, tabWrapperRef }
}

function RailPanel({
  isReviewPanelOpen,
  isHistoryView,
  railTabs,
  focusMode,
}: {
  isReviewPanelOpen: boolean
  isHistoryView: boolean
  railTabs: RailElement[]
  focusMode: boolean
}) {
  const { selectedTab, panelRef, handlePaneExpand, handlePaneCollapse } = useRailContext()
  const prevTab = usePrevious(selectedTab)
  const tabHasChanged = prevTab !== selectedTab

  const onCollapse = useCallback(() => {
    if (!tabHasChanged) {
      handlePaneCollapse()
    }
  }, [tabHasChanged, handlePaneCollapse])

  // A tab's panel is built the first time it is shown and kept after that,
  // so switching back finds it as it was left.
  const [mounted, setMounted] = useState<Set<string>>(() => new Set(['file-tree']))
  useEffect(() => {
    setMounted(previous => {
      if (previous.has(selectedTab)) {
        return previous
      }
      const next = new Set(previous)
      next.add(selectedTab)
      return next
    })
  }, [selectedTab])

  return (
    <Panel
      id={`ide-redesign-sidebar-panel-${isHistoryView ? 'file-tree' : selectedTab}`}
      className={isReviewPanelOpen || focusMode ? 'hidden' : undefined}
      order={1}
      defaultSize={selectedTab === 'workbench' ? 20 : 15}
      minSize={5}
      maxSize={80}
      ref={panelRef}
      collapsible
      onCollapse={onCollapse}
      onExpand={handlePaneExpand}
    >
      {isHistoryView ? <nav id="history-file-tree" className="ide-react-editor-sidebar history-file-tree" /> : null}
      <div className={['ide-rail-content', isHistoryView ? 'hidden' : ''].filter(Boolean).join(' ')}>
        <div className="tab-content ide-rail-tab-content">
          {railTabs
            .filter(shouldIncludeElement)
            .filter(tab => tab.mountOnFirstLoad || mounted.has(tab.key))
            .map(({ key, component }) => (
              <div key={key} role="tabpanel" className={['tab-pane', selectedTab === key ? 'active' : ''].filter(Boolean).join(' ')}>
                {component}
              </div>
            ))}
        </div>
      </div>
    </Panel>
  )
}

function RailResizeHandle({ isReviewPanelOpen, focusMode }: { isReviewPanelOpen: boolean; focusMode: boolean }) {
  const { isOpen, setIsOpen, togglePane, setResizing } = useRailContext()
  const { t } = useTranslation()

  return (
    <HorizontalResizeHandle
      className={isReviewPanelOpen || focusMode ? 'hidden' : undefined}
      resizable
      hitAreaMargins={{ coarse: 0, fine: 0 }}
      onDoubleClick={togglePane}
      onDragging={setResizing}
    >
      <HorizontalToggler
        id="ide-redesign-sidebar-panel"
        togglerType="west"
        isOpen={isOpen}
        setIsOpen={setIsOpen}
        tooltipWhenOpen={t('tooltip_hide_panel')}
        tooltipWhenClosed={t('tooltip_show_panel')}
      />
    </HorizontalResizeHandle>
  )
}

function RailModals() {
  const { activeModal, setActiveModal } = useRailContext()
  const { features } = useProject()
  return (
    <HotkeysModal
      show={activeModal === 'keyboard-shortcuts'}
      handleHide={() => setActiveModal(null)}
      isMac={isMac}
      trackChangesVisible={features.trackChangesVisible}
    />
  )
}

export function RailPanelBody({ children }: { children: ReactNode }) {
  return <>{children}</>
}
