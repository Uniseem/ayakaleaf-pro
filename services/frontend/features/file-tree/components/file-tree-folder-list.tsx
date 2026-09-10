'use client'

import cx from '@/lib/cx'
import FileTreeDoc from './file-tree-doc'
import FileTreeFolder from './file-tree-folder'
import { fileCollator } from '../util/file-collator'
import type { TreeNode } from '../tree'

/** One level of the tree: folders first, then everything else, each by name. */
export function FileTreeFolderList({
  nodes,
  className,
  dropProps,
  children,
  dataTestId,
}: {
  nodes: TreeNode[]
  className?: string
  dropProps?: Record<string, unknown>
  children?: React.ReactNode
  dataTestId?: string
}) {
  const folders = nodes.filter(node => node.entry.kind === 'folder').sort(compare)
  const docsAndFiles = nodes.filter(node => node.entry.kind !== 'folder').sort(compare)

  return (
    <ul
      className={cx('list-unstyled', 'file-tree-folder-list', className)}
      role="tree"
      data-testid={dataTestId}
      {...dropProps}
    >
      <div className="file-tree-folder-list-inner">
        {folders.map(node => (
          <FileTreeFolder
            key={node.entry.id}
            id={node.entry.id}
            name={node.entry.name}
            children={node.children}
          />
        ))}
        {docsAndFiles.map(node => (
          <FileTreeDoc
            key={node.entry.id}
            id={node.entry.id}
            name={node.entry.name}
            isFile={node.entry.kind === 'file'}
          />
        ))}
        {children}
      </div>
    </ul>
  )
}

function compare(one: TreeNode, two: TreeNode) {
  return fileCollator.compare(one.entry.name, two.entry.name)
}

export default FileTreeFolderList
