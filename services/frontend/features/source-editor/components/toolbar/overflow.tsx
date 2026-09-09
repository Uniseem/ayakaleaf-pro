'use client'

import { useCallback, useEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from '@/lib/i18n'
import cx from '@/lib/cx'
import MaterialIcon from '@/components/ol/material-icon'
import { useCodeMirrorViewContext } from '../codemirror-context'
import { Overlay, Popover } from '@/components/ol/overlay'

export const ToolbarOverflow = ({
  overflowed,
  overflowOpen,
  setOverflowOpen,
  overflowRef,
  popoverClassName,
  children,
}: {
  overflowed: boolean
  overflowOpen: boolean
  setOverflowOpen: (open: boolean) => void
  overflowRef?: (node: Element | null) => void
  popoverClassName?: string
  children: ReactNode
}) => {
  const { t } = useTranslation()
  const buttonRef = useRef<HTMLButtonElement>(null)
  const keyboardInputRef = useRef(false)
  const view = useCodeMirrorViewContext()

  const className = cx('ol-cm-toolbar-button', 'ol-cm-toolbar-overflow-toggle', {
    'ol-cm-toolbar-overflow-toggle-visible': overflowed,
  })

  // A11y - Move the focus inside the popover to the first toolbar button when it opens
  const handlePopoverFocus = useCallback(() => {
    if (keyboardInputRef.current) {
      const firstToolbarItem = document.querySelector(
        '#popover-toolbar-overflow .ol-cm-toolbar-overflow button:not([disabled])'
      ) as HTMLButtonElement | null

      if (firstToolbarItem) {
        firstToolbarItem.focus()
      }
    }
  }, [])

  const handleKeyDown = useCallback(() => {
    keyboardInputRef.current = true
  }, [])

  const handleMouseDown = useCallback(() => {
    keyboardInputRef.current = false
  }, [])

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    document.addEventListener('mousedown', handleMouseDown)

    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      document.removeEventListener('mousedown', handleMouseDown)
    }
  }, [handleKeyDown, handleMouseDown])

  // A11y - Move the focus back to the trigger when the popover is dismissed
  const handleCloseAndReturnFocus = useCallback(() => {
    setOverflowOpen(false)

    if (keyboardInputRef.current && buttonRef.current) {
      buttonRef.current.focus()
    }
  }, [setOverflowOpen])

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        id="toolbar-more"
        className={className}
        aria-label={t('more_editor_toolbar_item')}
        aria-expanded={overflowOpen}
        aria-controls="popover-toolbar-overflow"
        onMouseDown={event => {
          event.preventDefault()
          event.stopPropagation()
        }}
        onClick={() => {
          setOverflowOpen(!overflowOpen)
        }}
      >
        <MaterialIcon type="more_horiz" />
      </button>

      <Overlay
        show={overflowOpen}
        target={buttonRef.current}
        placement="bottom"
        container={view.dom}
        transition
        rootClose
        onHide={handleCloseAndReturnFocus}
        onEntered={handlePopoverFocus}
      >
        <Popover id="popover-toolbar-overflow" ref={overflowRef} role="toolbar">
          <div className={cx(popoverClassName, 'ol-cm-toolbar-overflow')}>{children}</div>
        </Popover>
      </Overlay>
    </>
  )
}
