'use client'

/**
 * One row's contents, from file-tree-item/file-tree-item-inner.
 *
 * A folder's name is a button because clicking it opens the folder; a file's
 * is not, because the click is handled by the row itself and a button inside
 * a clickable row is one control too many for a screen reader.
 */

import { useEffect, useRef, type ReactNode } from 'react'
import cx from '@/lib/cx'
import { useProject } from '@/features/ide/contexts/project-context'
import { useFileTreeMainContext } from '../../contexts/file-tree-main'
import { useDraggable } from '../../contexts/file-tree-draggable'
import { useFileTreeSelectable } from '../../contexts/file-tree-selectable'
import { useFileTreeActionable } from '../../contexts/file-tree-actionable'
import FileTreeItemName from './file-tree-item-name'
import FileTreeItemMenu from './file-tree-item-menu'

export function FileTreeItemInner({
  id,
  name,
  type,
  isSelected,
  icons,
  onClick,
}: {
  id: string
  name: string
  type: 'doc' | 'file' | 'folder'
  isSelected: boolean
  icons?: ReactNode
  onClick?: () => void
}) {
  const { canWrite } = useProject()
  const { setContextMenuCoords } = useFileTreeMainContext()
  const { isRenaming } = useFileTreeActionable()
  const { selectedEntityIds } = useFileTreeSelectable()

  const hasMenu = canWrite && isSelected && selectedEntityIds.size === 1

  const { dragProps, setIsDraggable, isBeingDragged } = useDraggable(id)

  const itemRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const item = itemRef.current
    if (isSelected && item) {
      // Delayed, because the outline below shares this panel and resizes after
      // a selection: scrolling immediately puts the row in view and the resize
      // then pushes it out again.
      const timer = window.setTimeout(() => {
        item.scrollIntoView({ block: 'nearest', inline: 'nearest' })
      }, 100)
      return () => window.clearTimeout(timer)
    }
  }, [isSelected])

  function handleContextMenu(event: React.MouseEvent<HTMLDivElement>) {
    // Shift is how somebody asks for the browser's own menu.
    if (event.shiftKey) {
      setContextMenuCoords(null)
      return
    }

    event.preventDefault()

    setContextMenuCoords({ top: event.pageY, left: event.pageX })
  }

  return (
    <div
      className={cx('entity', { 'file-tree-entity-dragging': isBeingDragged })}
      role="presentation"
      {...dragProps}
      draggable={dragProps.draggable && !isRenaming}
      onContextMenu={handleContextMenu}
      data-file-id={id}
      data-file-type={type}
    >
      <div className="entity-name entity-name-react" role="presentation" ref={itemRef}>
        <FileTreeItemIconsAndName
          name={name}
          isSelected={isSelected}
          icons={icons}
          onClick={onClick}
          setIsDraggable={setIsDraggable}
        />
        {hasMenu ? <FileTreeItemMenu id={id} name={name} /> : null}
      </div>
    </div>
  )
}

function FileTreeItemIconsAndName({
  name,
  isSelected,
  icons,
  onClick,
  setIsDraggable,
}: {
  name: string
  isSelected: boolean
  icons?: ReactNode
  onClick?: () => void
  setIsDraggable: (isDraggable: boolean) => void
}) {
  return onClick ? (
    <button className="file-tree-entity-button" onClick={onClick}>
      {icons}
      <FileTreeItemName name={name} isSelected={isSelected} setIsDraggable={setIsDraggable} />
    </button>
  ) : (
    <div className="file-tree-entity-details">
      {icons}
      <FileTreeItemName name={name} isSelected={isSelected} setIsDraggable={setIsDraggable} />
    </div>
  )
}

export default FileTreeItemInner
