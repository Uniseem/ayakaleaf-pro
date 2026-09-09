'use client'

import { memo, useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { getPanel } from '@codemirror/view'
import { language } from '@codemirror/language'
import { useCodeMirrorStateContext, useCodeMirrorViewContext } from './codemirror-context'
import { useResizeObserver } from '../hooks/use-resize-observer'
import { ToolbarItems } from './toolbar/toolbar-items'
import { ToolbarOverflow } from './toolbar/overflow'
import useDropdown from '../hooks/use-dropdown'
import { createToolbarPanel } from '../extensions/toolbar/toolbar-panel'
import EditorSwitch from './editor-switch'
import SwitchToPDFButton from './switch-to-pdf-button'
import { isVisual } from '../extensions/visual/visual'
import { minimumListDepthForSelection } from '../utils/tree-operations/ancestors'
import { debugConsole } from '@/lib/debug'
import { useTranslation } from '@/lib/i18n'
import { ToggleSearchButton } from './toolbar/toggle-search-button'
import Breadcrumbs from './breadcrumbs'
import cx from '@/lib/cx'
import { useSettings } from '@/features/ide/contexts/settings-context'
import { useLayout } from '@/features/ide/contexts/layout-context'

export const CodeMirrorToolbar = () => {
  const view = useCodeMirrorViewContext()
  const panel = getPanel(view, createToolbarPanel)

  if (!panel) {
    return null
  }

  return createPortal(<Toolbar />, panel.dom)
}

const Toolbar = memo(function Toolbar() {
  const { t } = useTranslation()
  const state = useCodeMirrorStateContext()
  const view = useCodeMirrorViewContext()
  const { breadcrumbs } = useSettings()
  const { focusMode } = useLayout()

  const [overflowed, setOverflowed] = useState(false)

  const overflowedItemsRef = useRef<Set<string>>(new Set())

  const languageName = state.facet(language)?.name
  const visual = isVisual(view)

  const listDepth = minimumListDepthForSelection(state)

  // the review panel header belongs to the review phase
  const showReviewPanelHeader = false

  const { open: overflowOpen, onToggle: setOverflowOpen, ref: overflowRef } = useDropdown()

  const buildOverflow = useCallback(
    (element: Element) => {
      debugConsole.log('recalculating toolbar overflow')

      setOverflowOpen(false)
      setOverflowed(true)

      overflowedItemsRef.current = new Set()

      const buttonGroups = [...element.querySelectorAll<HTMLDivElement>('[data-overflow]')].reverse()

      // restore all the overflowed items
      for (const buttonGroup of buttonGroups) {
        buttonGroup.classList.remove('overflow-hidden')
      }

      // find all the available items
      for (const buttonGroup of buttonGroups) {
        if (element.scrollWidth <= element.clientWidth) {
          break
        }
        // add this item to the overflow
        overflowedItemsRef.current.add(buttonGroup.dataset.overflow!)
        buttonGroup.classList.add('overflow-hidden')
      }

      setOverflowed(overflowedItemsRef.current.size > 0)
    },
    [setOverflowOpen]
  )

  // calculate overflow when the container resizes
  const { elementRef, resizeRef } = useResizeObserver(buildOverflow)

  // calculate overflow when `languageName` or `visual` change
  useEffect(() => {
    if (resizeRef.current) {
      buildOverflow(resizeRef.current.element)
    }
  }, [buildOverflow, languageName, listDepth, resizeRef, visual])

  // calculate overflow when toolbar content changes
  const observerRef = useRef<MutationObserver | null>(null)
  const handleToolbar = useCallback(
    (node: HTMLDivElement | null) => {
      // register the resize observer on the toolbar node
      elementRef(node)

      if (observerRef.current) {
        observerRef.current.disconnect()
        observerRef.current = null
      }

      if (!('MutationObserver' in window)) {
        return
      }

      if (node) {
        observerRef.current = new MutationObserver(() => {
          if (resizeRef.current) {
            buildOverflow(resizeRef.current.element)
          }
        })

        observerRef.current.observe(node, { childList: true, subtree: true })
      }
    },
    [buildOverflow, elementRef, resizeRef]
  )

  // calculate overflow when active element changes to/from inside a table
  const insideTable = document.activeElement?.closest('.table-generator-help-modal,.table-generator,.table-generator-width-modal')
  useEffect(() => {
    if (resizeRef.current) {
      buildOverflow(resizeRef.current.element)
    }
  }, [buildOverflow, insideTable, resizeRef])

  const showActions = !state.readOnly && !insideTable

  if (focusMode) {
    return null
  }

  return (
    <>
      <div
        id="ol-cm-toolbar-wrapper"
        className={cx('ol-cm-toolbar-wrapper', {
          'ol-cm-toolbar-wrapper-indented': showReviewPanelHeader,
        })}
      >
        <div role="toolbar" aria-label={t('toolbar_editor')} className="ol-cm-toolbar toolbar-editor" ref={handleToolbar}>
          {showActions && <ToolbarItems state={state} languageName={languageName} visual={visual} listDepth={listDepth} />}

          <div className="ol-cm-toolbar-button-group ol-cm-toolbar-stretch">
            {showActions && (
              <ToolbarOverflow
                overflowed={overflowed}
                overflowOpen={overflowOpen}
                setOverflowOpen={setOverflowOpen}
                overflowRef={overflowRef}
              >
                <ToolbarItems
                  state={state}
                  overflowed={overflowedItemsRef.current}
                  languageName={languageName}
                  visual={visual}
                  listDepth={listDepth}
                />
              </ToolbarOverflow>
            )}
          </div>

          <div className="ol-cm-toolbar-button-group ol-cm-toolbar-end">
            <EditorSwitch />
            <ToggleSearchButton state={state} />
            <SwitchToPDFButton />
          </div>
        </div>
        {breadcrumbs && <Breadcrumbs />}
      </div>
    </>
  )
})
