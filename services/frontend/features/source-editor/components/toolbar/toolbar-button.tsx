'use client'

import { memo, useCallback } from 'react'
import type { EditorView } from '@codemirror/view'
import { useCodeMirrorViewContext } from '../codemirror-context'
import cx from '@/lib/cx'
import { emitToolbarEvent } from '../../extensions/toolbar/utils/analytics'
import MaterialIcon from '@/components/ol/material-icon'
import { Tooltip } from '@/components/ol/tooltip'

export const ToolbarButton = memo(function ToolbarButton({
  id,
  className,
  label,
  command,
  active = false,
  disabled,
  icon,
  textIcon = false,
  hidden = false,
  shortcut,
}: {
  id: string
  className?: string
  label: string
  command?: (view: EditorView) => void
  active?: boolean
  disabled?: boolean
  icon: string
  textIcon?: boolean
  hidden?: boolean
  shortcut?: string
}) {
  const view = useCodeMirrorViewContext()

  const handleMouseDown = useCallback((event: React.MouseEvent) => {
    event.preventDefault()
  }, [])

  const handleClick = useCallback(
    (event: React.MouseEvent) => {
      emitToolbarEvent(view, id)
      if (command) {
        event.preventDefault()
        command(view)
        view.focus()
      }
    },
    [command, view, id]
  )

  const button = (
    <button
      className={cx('ol-cm-toolbar-button', className, {
        active,
        hidden,
      })}
      aria-label={label}
      onMouseDown={handleMouseDown}
      onClick={!disabled ? handleClick : undefined}
      aria-disabled={disabled}
      type="button"
    >
      {textIcon ? icon : <MaterialIcon type={icon} accessibilityLabel={label} />}
    </button>
  )

  if (!label) {
    return button
  }

  const description = (
    <>
      <div>
        {label}
        {shortcut && <span> ({shortcut})</span>}
      </div>
    </>
  )

  return (
    <Tooltip id={id} description={description} overlayProps={{ placement: 'bottom' }}>
      {button}
    </Tooltip>
  )
})
