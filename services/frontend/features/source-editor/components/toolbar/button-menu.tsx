'use client'

import { memo, useEffect, useRef, type ReactNode } from 'react'
import type { EditorView } from '@codemirror/view'
import useDropdown from '../../hooks/use-dropdown'
import { ListGroup } from '@/components/ol/list-group'
import { Tooltip } from '@/components/ol/tooltip'
import { Overlay, Popover } from '@/components/ol/overlay'
import { emitToolbarEvent } from '../../extensions/toolbar/utils/analytics'
import { useCodeMirrorViewContext } from '../codemirror-context'
import cx from '@/lib/cx'

export const ToolbarButtonMenu = memo(function ButtonMenu({
  icon,
  id,
  label,
  orientation = 'vertical',
  className,
  altCommand,
  onToggle,
  disabled,
  disablePopover,
  children,
}: {
  id: string
  label: string
  icon: ReactNode
  orientation?: 'vertical' | 'horizontal'
  className?: string
  disabled?: boolean
  disablePopover?: boolean
  altCommand?: (view: EditorView) => void
  onToggle?: (isOpen: boolean) => void
  children: ReactNode
}) {
  const target = useRef<HTMLButtonElement>(null)
  const { open, onToggle: handleToggle, ref } = useDropdown()
  const view = useCodeMirrorViewContext()

  useEffect(() => {
    if (!open) return

    const onResize = () => {
      handleToggle(false)
    }
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
    }
  }, [open, handleToggle])

  useEffect(() => {
    if (disablePopover && open) {
      handleToggle(false)
    }
  }, [open, disablePopover, handleToggle])

  useEffect(() => {
    onToggle?.(open)
  }, [open, onToggle])

  const button = (
    <button
      type="button"
      className={cx('ol-cm-toolbar-button', className)}
      aria-label={label}
      aria-disabled={disabled}
      aria-haspopup="menu"
      aria-expanded={open}
      aria-controls={`${id}-menu`}
      onMouseDown={event => {
        event.preventDefault()
        event.stopPropagation()
      }}
      onClick={event => {
        if (disabled) {
          event.preventDefault()
          return
        }
        if (event.altKey && altCommand && open === false) {
          emitToolbarEvent(view, id)
          event.preventDefault()
          altCommand(view)
          view.focus()
        } else {
          handleToggle(!open)
        }
      }}
      ref={target}
    >
      {icon}
    </button>
  )

  const overlay = (
    <Overlay
      show={open && !disablePopover}
      target={target.current}
      placement="bottom"
      container={view.dom}
      containerPadding={0}
      transition
      rootClose
      onHide={() => handleToggle(false)}
    >
      <Popover
        id={`${id}-menu`}
        ref={ref}
        className={cx('ol-cm-toolbar-button-menu-popover', {
          'ol-cm-toolbar-button-menu-popover-horizontal': orientation === 'horizontal',
        })}
      >
        <ListGroup
          role="menu"
          onClick={() => {
            handleToggle(false)
          }}
        >
          {children}
        </ListGroup>
      </Popover>
    </Overlay>
  )

  if (!label) {
    return (
      <>
        {button}
        {overlay}
      </>
    )
  }

  return (
    <>
      <Tooltip hidden={open} id={id} description={<div>{label}</div>} overlayProps={{ placement: 'bottom' }}>
        {button}
      </Tooltip>
      {overlay}
    </>
  )
})
