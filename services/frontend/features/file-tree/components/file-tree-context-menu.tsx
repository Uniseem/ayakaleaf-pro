'use client'

/**
 * The right-click menu, from file-tree/components/file-tree-context-menu.
 *
 * Drawn into the body at the pointer's page coordinates, so it is not clipped
 * by the tree's own scrolling. It opens upwards when there is not enough room
 * below, which is the difference between a menu and a menu you cannot reach.
 */

import { useCallback, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import cx from '@/lib/cx'
import { Dropdown, DropdownMenu } from '@/components/ol/dropdown'
import { useProject } from '@/features/ide/contexts/project-context'
import { useFileTreeMainContext } from '../contexts/file-tree-main'
import FileTreeItemMenuItems from './file-tree-item/file-tree-item-menu-items'

export function FileTreeContextMenu() {
  const { canWrite } = useProject()
  const { contextMenuCoords, setContextMenuCoords } = useFileTreeMainContext()
  const toggleButtonRef = useRef<HTMLButtonElement | null>(null)
  const keyboardInputRef = useRef(false)

  useEffect(() => {
    if (contextMenuCoords) {
      toggleButtonRef.current = document.querySelector('.entity-menu-toggle')
    }
  }, [contextMenuCoords])

  // Opened from the keyboard, the first item takes the focus; opened by the
  // pointer it does not, or the menu would steal the caret mid-click.
  useEffect(() => {
    if (contextMenuCoords && keyboardInputRef.current) {
      const first = document.querySelector<HTMLButtonElement>(
        '#dropdown-file-tree-context-menu .dropdown-item:not([disabled])'
      )
      first?.focus()
    }
  }, [contextMenuCoords])

  const close = useCallback(() => {
    setContextMenuCoords(null)
    // The focus goes back where it came from, as a menu should leave it.
    toggleButtonRef.current?.focus()
  }, [setContextMenuCoords])

  function handleToggle(wantOpen: boolean) {
    if (!wantOpen) {
      close()
    }
  }

  function handleClose(event: React.KeyboardEvent<Element>) {
    if (event.key === 'Tab' || event.key === 'Escape') {
      event.preventDefault()
      close()
    }
  }

  const handleKeyDown = useCallback(() => {
    keyboardInputRef.current = true
  }, [])

  const handleMouseDown = useCallback(() => {
    keyboardInputRef.current = false
  }, [])

  const handleShiftContextMenu = useCallback(
    (event: MouseEvent) => {
      if (event.shiftKey) {
        setContextMenuCoords(null)
      }
    },
    [setContextMenuCoords]
  )

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    document.addEventListener('mousedown', handleMouseDown)
    document.addEventListener('contextmenu', handleShiftContextMenu)

    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      document.removeEventListener('mousedown', handleMouseDown)
      document.removeEventListener('contextmenu', handleShiftContextMenu)
    }
  }, [handleKeyDown, handleMouseDown, handleShiftContextMenu])

  if (!contextMenuCoords || !canWrite || typeof document === 'undefined') {
    return null
  }

  const dropDirection =
    document.body.offsetHeight / contextMenuCoords.top < 2 &&
    document.body.offsetHeight - contextMenuCoords.top < 250
      ? 'up'
      : 'down'

  return createPortal(
    <div style={contextMenuCoords} className="context-menu ide-redesign-main">
      <Dropdown show drop={dropDirection} onKeyDown={handleClose} onToggle={handleToggle}>
        <DropdownMenu
          className={cx('dropdown-menu-sm-width', {
            // Positioned by absolute coordinates rather than against a toggle,
            // so the upward case has to be named rather than inferred.
            'context-menu-upwards': dropDirection === 'up',
          })}
          id="dropdown-file-tree-context-menu"
        >
          <FileTreeItemMenuItems />
        </DropdownMenu>
      </Dropdown>
    </div>,
    document.body
  )
}

export default FileTreeContextMenu
