'use client'

import { useCallback, useEffect } from 'react'
import cx from '@/lib/cx'
import { usePersistedState } from '@/lib/hooks'
import { useFileTreeSelectable, useSelectableEntity } from '../contexts/file-tree-selectable'
import { useDroppable } from '../contexts/file-tree-draggable'
import FileTreeItemInner from './file-tree-item/file-tree-item-inner'
import FileTreeFolderList from './file-tree-folder-list'
import FileTreeFolderIcons from './file-tree-folder-icons'
import type { TreeNode } from '../tree'

/** A folder row, and its contents when it is open. */
export function FileTreeFolder({ id, name, children }: { id: string; name: string; children: TreeNode[] }) {
  const { isSelected, props: selectableEntityProps } = useSelectableEntity(id, 'folder')

  const { selectedEntityParentIds } = useFileTreeSelectable()

  const [expanded, setExpanded] = usePersistedState(`folder.${id}.expanded`, false)

  // A folder holding the open file opens itself, or the selection would be
  // somewhere the reader cannot see.
  useEffect(() => {
    if (selectedEntityParentIds.has(id)) {
      setExpanded(true)
    }
  }, [id, selectedEntityParentIds, setExpanded])

  const handleExpandCollapseClick = useCallback(() => {
    setExpanded(value => !value)
  }, [setExpanded])

  const { isOver: isOverRoot, dropProps: dropPropsRoot } = useDroppable(id)
  const { isOver: isOverList, dropProps: dropPropsList } = useDroppable(id)

  return (
    <>
      <li
        role="treeitem"
        {...selectableEntityProps}
        {...dropPropsRoot}
        aria-expanded={expanded}
        aria-label={name}
        className={cx(selectableEntityProps.className, {
          'dnd-droppable-hover': isOverRoot || isOverList,
        })}
        translate="no"
      >
        <FileTreeItemInner
          id={id}
          name={name}
          type="folder"
          isSelected={isSelected}
          onClick={handleExpandCollapseClick}
          icons={<FileTreeFolderIcons expanded={expanded} />}
        />
      </li>
      {expanded ? <FileTreeFolderList nodes={children} dropProps={dropPropsList} /> : null}
    </>
  )
}

export default FileTreeFolder
