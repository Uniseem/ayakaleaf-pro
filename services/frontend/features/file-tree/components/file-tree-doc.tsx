'use client'

import { useCallback } from 'react'
import { useSelectableEntity } from '../contexts/file-tree-selectable'
import FileTreeIcon from './file-tree-icon'
import FileTreeItemInner from './file-tree-item/file-tree-item-inner'
import { useTabsContext } from '@/features/ide/contexts/tabs-context'

/** A file or a document: one row of the tree. */
export function FileTreeDoc({
  name,
  id,
  isFile,
  isLinkedFile,
}: {
  name: string
  id: string
  isFile?: boolean
  isLinkedFile?: boolean
}) {
  const type = isFile ? 'file' : 'doc'

  const { isSelected, props: selectableEntityProps } = useSelectableEntity(id, type)

  const { makeTabPermanent } = useTabsContext()

  // A single click previews; a double click is somebody saying they mean to
  // stay, so the tab stops being the temporary one.
  const onDoubleClick = useCallback(() => {
    makeTabPermanent(id)
  }, [makeTabPermanent, id])

  return (
    <li
      role="treeitem"
      {...selectableEntityProps}
      aria-label={name}
      tabIndex={0}
      translate="no"
      onDoubleClick={onDoubleClick}
    >
      <FileTreeItemInner
        id={id}
        name={name}
        type={type}
        isSelected={isSelected}
        icons={<FileTreeIcon isLinkedFile={isLinkedFile} name={name} />}
      />
    </li>
  )
}

export default FileTreeDoc
