'use client'

/**
 * The row of open-file tabs, from source-editor/components/tabs.
 *
 * Tabs can be dragged into a new order, closed with their own button or a
 * middle click, and right-clicked for a menu that closes the others or the
 * ones to the right.
 */

import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { Dropdown, DropdownDivider, DropdownItem, DropdownMenu } from '@/components/ol/dropdown'
import { TAB_TRANSFER_TYPE, useTabsContext, type EditorFileTab } from '@/features/ide/contexts/tabs-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { EntryIcon } from '@/features/file-tree/entry-icon'

function throttle<A extends unknown[]>(fn: (...args: A) => void, wait: number) {
  let timer: number | null = null
  let last: A | null = null
  const wrapped = (...args: A) => {
    last = args
    if (timer === null) {
      timer = window.setTimeout(() => {
        timer = null
        if (last) {
          fn(...last)
        }
      }, wait)
    }
  }
  wrapped.cancel = () => {
    if (timer !== null) {
      window.clearTimeout(timer)
      timer = null
    }
    last = null
  }
  return wrapped
}

export function TabsContainer() {
  const { tabs, openTab, closeTab, moveTab, makeTabPermanent, setContextMenuTarget, setHeaderSlot } = useTabsContext()
  const editor = useEditor()
  const openEntityId = editor.currentFile?.id ?? editor.current?.id ?? null
  const [hovered, setHovered] = useState(false)

  const openContextMenu = useCallback(
    (coords: { top: number; left: number }, tabId: string) => {
      setContextMenuTarget({ ...coords, tabId })
    },
    [setContextMenuTarget]
  )

  const closeContextMenu = useCallback(() => setContextMenuTarget(null), [setContextMenuTarget])

  const throttledOnDragOver = useMemo(() => throttle(() => setHovered(true), 50), [])

  const onDragOver = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()
      throttledOnDragOver()
    },
    [throttledOnDragOver]
  )

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      throttledOnDragOver.cancel()
      event.stopPropagation()
      event.preventDefault()
      setHovered(false)
      const draggedTabId = event.dataTransfer.getData(TAB_TRANSFER_TYPE)
      if (!draggedTabId) {
        return
      }
      const targetTabId = tabs[tabs.length - 1]?.id
      if (!targetTabId) {
        return
      }
      moveTab(draggedTabId, targetTabId, 'right')
    },
    [tabs, moveTab, throttledOnDragOver]
  )

  const onDragLeave = useCallback(() => {
    throttledOnDragOver.cancel()
    setHovered(false)
  }, [throttledOnDragOver])

  const onWheel = useCallback((event: React.WheelEvent) => {
    if (event.deltaY !== 0 && event.deltaX === 0) {
      event.currentTarget.scrollLeft += event.deltaY
    }
  }, [])

  return (
    <div className="editor-tabs-container">
      <div className="review-panel-header-slot" ref={setHeaderSlot} />
      <div
        className={['editor-tabs-row', hovered ? 'editor-tabs-row-hovered' : ''].filter(Boolean).join(' ')}
        role="tablist"
        onDragOver={onDragOver}
        onDrop={onDrop}
        onDragLeave={onDragLeave}
        onWheel={onWheel}
        tabIndex={-1}
      >
        {tabs.map(tab => (
          <Tab
            key={tab.id}
            tab={tab}
            openTab={openTab}
            closeTab={closeTab}
            canCloseTab={tabs.length > 1}
            isSelected={openEntityId === tab.id}
            onTabDrop={moveTab}
            makeTabPermanent={makeTabPermanent}
            openContextMenu={openContextMenu}
            closeContextMenu={closeContextMenu}
          />
        ))}
      </div>
      <div className="editor-tabs-labs-icon" />
      <TabsContextMenu />
    </div>
  )
}

function getSideOfTargetFromEvent(element: Element, clientX: number): 'left' | 'right' {
  const rect = element.getBoundingClientRect()
  return rect.left + rect.width / 2 < clientX ? 'right' : 'left'
}

const Tab = memo(function Tab({
  tab,
  openTab,
  closeTab,
  canCloseTab,
  makeTabPermanent,
  openContextMenu,
  closeContextMenu,
  isSelected,
  onTabDrop,
}: {
  tab: EditorFileTab
  openTab: (id: string) => void
  closeTab: (id: string) => void
  canCloseTab: boolean
  makeTabPermanent: (id: string) => void
  openContextMenu: (coords: { top: number; left: number }, tabId: string) => void
  closeContextMenu: () => void
  isSelected: boolean
  onTabDrop: (sourceTabId: string, targetTabId: string, position: 'left' | 'right') => void
}) {
  const { t } = useTranslation()
  const tabRef = useRef<HTMLDivElement>(null)
  const [dropTargetPosition, setDropTargetPosition] = useState<'left' | 'right' | null>(null)

  const onDragStart = useCallback(
    (event: React.DragEvent) => {
      event.stopPropagation()
      event.dataTransfer.setData(TAB_TRANSFER_TYPE, tab.id)
      event.dataTransfer.effectAllowed = 'move'
    },
    [tab]
  )

  const throttledOnDragOver = useMemo(
    () =>
      throttle((element: Element, clientX: number) => {
        setDropTargetPosition(getSideOfTargetFromEvent(element, clientX))
      }, 50),
    []
  )

  const onDragOver = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()
      event.stopPropagation()
      event.dataTransfer.dropEffect = 'move'
      throttledOnDragOver(event.currentTarget, event.clientX)
    },
    [throttledOnDragOver]
  )

  const onDragLeave = useCallback(
    (event: React.DragEvent) => {
      event.stopPropagation()
      if (event.currentTarget.contains(event.relatedTarget as Node | null)) {
        return
      }
      throttledOnDragOver.cancel()
      setDropTargetPosition(null)
    },
    [throttledOnDragOver]
  )

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      throttledOnDragOver.cancel()
      setDropTargetPosition(null)
      event.preventDefault()
      event.stopPropagation()
      const draggedTabId = event.dataTransfer.getData(TAB_TRANSFER_TYPE)
      if (!draggedTabId) {
        return
      }
      onTabDrop(draggedTabId, tab.id, getSideOfTargetFromEvent(event.currentTarget, event.clientX))
    },
    [onTabDrop, tab, throttledOnDragOver]
  )

  const onDoubleClick = useCallback(() => makeTabPermanent(tab.id), [makeTabPermanent, tab])

  const onClick = useCallback(
    (event: React.MouseEvent) => {
      if (event.button === 0) {
        openTab(tab.id)
      }
    },
    [openTab, tab]
  )

  const onKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.code === 'Enter') {
        openTab(tab.id)
      }
    },
    [openTab, tab]
  )

  const onCloseClick = useCallback(
    (event: React.MouseEvent) => {
      event.stopPropagation()
      closeTab(tab.id)
    },
    [closeTab, tab]
  )

  const onMouseUp = useCallback(
    (event: React.MouseEvent) => {
      if (event.button === 1) {
        event.stopPropagation()
        closeTab(tab.id)
      }
    },
    [closeTab, tab]
  )

  const onContextMenu = useCallback(
    (event: React.MouseEvent) => {
      if (event.shiftKey) {
        closeContextMenu()
        return
      }
      event.preventDefault()
      openContextMenu({ top: event.pageY, left: event.pageX }, tab.id)
    },
    [openContextMenu, closeContextMenu, tab]
  )

  useLayoutEffect(() => {
    if (isSelected && tabRef.current) {
      tabRef.current.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    }
  }, [isSelected])

  // An edit in a temporary tab makes it permanent.
  useEffect(() => {
    if (isSelected && tab.lifetime === 'temporary') {
      const handler = () => makeTabPermanent(tab.id)
      window.addEventListener('ide:user-edit', handler)
      return () => window.removeEventListener('ide:user-edit', handler)
    }
  }, [isSelected, makeTabPermanent, tab])

  return (
    <div
      ref={tabRef}
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      draggable
      onDrop={onDrop}
      onDoubleClick={onDoubleClick}
      role="tab"
      aria-selected={isSelected}
      onClick={onClick}
      onMouseUp={onMouseUp}
      onKeyDown={onKeyDown}
      onContextMenu={onContextMenu}
      data-tab-id={tab.id}
      tabIndex={0}
      className={[
        'editor-file-tab',
        isSelected ? 'tab-selected' : '',
        dropTargetPosition === 'left' ? 'tab-drop-left' : '',
        dropTargetPosition === 'right' ? 'tab-drop-right' : '',
        tab.lifetime === 'temporary' ? 'tab-temporary' : '',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <div className="editor-file-tab-content">
        <span className="editor-file-tab-icon">
          <EntryIcon kind={tab.name.match(/\.(tex|bib|txt|md|cls|sty|bst|py)$/i) ? 'doc' : 'file'} name={tab.name} />
        </span>
        <div className="editor-file-tab-path">&lrm;{tab.displayPath}</div>
        <div className="editor-file-tab-action">
          <button type="button" onClick={onCloseClick} disabled={!canCloseTab} className="editor-file-tab-close-action" aria-label={t('close')}>
            <MaterialIcon type="close" />
          </button>
        </div>
      </div>
    </div>
  )
})

function TabsContextMenu() {
  const { t } = useTranslation()
  const { tabs, closeTab, closeOtherTabs, closeToRight, contextMenuTarget, setContextMenuTarget } = useTabsContext()
  const { setLeftMenuShown } = useLayout()
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!contextMenuTarget) return
    const handler = (event: MouseEvent) => {
      if (event.button !== 2) return
      const target = event.target as Element | null
      if (target?.closest('.context-menu')) return
      if (target?.closest('.editor-file-tab')) return
      setContextMenuTarget(null)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [contextMenuTarget, setContextMenuTarget])

  const lastTabIdRef = useRef<string | null>(null)
  useEffect(() => {
    if (contextMenuTarget) {
      lastTabIdRef.current = contextMenuTarget.tabId
      menuRef.current?.querySelector<HTMLElement>('[role="menu"]')?.focus()
    } else if (lastTabIdRef.current) {
      document.querySelector<HTMLElement>(`[data-tab-id="${lastTabIdRef.current}"]`)?.focus()
      lastTabIdRef.current = null
    }
  }, [contextMenuTarget])

  if (!contextMenuTarget || typeof document === 'undefined') return null

  const close = () => setContextMenuTarget(null)

  const handleKeyDown = (event: React.KeyboardEvent<Element>) => {
    if (event.key === 'Tab' || event.key === 'Escape') {
      event.preventDefault()
      close()
    }
  }

  return createPortal(
    <div ref={menuRef} style={{ position: 'absolute', top: contextMenuTarget.top, left: contextMenuTarget.left, zIndex: 1000 }} className="context-menu ide-redesign-main">
      <Dropdown show drop="down" onKeyDown={handleKeyDown} onToggle={wantOpen => (!wantOpen ? close() : undefined)}>
        <DropdownMenu className="dropdown-menu-sm-width" tabIndex={-1}>
          <li role="none">
            <DropdownItem
              disabled={tabs.length <= 1}
              as="button"
              onClick={() => {
                closeTab(contextMenuTarget.tabId)
                close()
              }}
            >
              {t('close')}
            </DropdownItem>
          </li>
          <li role="none">
            <DropdownItem
              disabled={tabs.length <= 1}
              as="button"
              onClick={() => {
                closeOtherTabs(contextMenuTarget.tabId)
                close()
              }}
            >
              {t('close_other_tabs')}
            </DropdownItem>
          </li>
          <li role="none">
            <DropdownItem
              disabled={tabs[tabs.length - 1]?.id === contextMenuTarget.tabId}
              as="button"
              onClick={() => {
                closeToRight(contextMenuTarget.tabId)
                close()
              }}
            >
              {t('close_tabs_to_the_right')}
            </DropdownItem>
          </li>
          <DropdownDivider />
          <li role="none">
            <DropdownItem
              as="button"
              onClick={() => {
                setLeftMenuShown(true)
                window.dispatchEvent(new CustomEvent('ui.focus-setting', { detail: 'editorTabs' }))
                close()
              }}
              leadingIcon="settings"
            >
              {t('tab_settings')}
            </DropdownItem>
          </li>
        </DropdownMenu>
      </Dropdown>
    </div>,
    document.body
  )
}
