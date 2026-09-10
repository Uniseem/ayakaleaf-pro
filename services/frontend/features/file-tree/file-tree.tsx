'use client'

/**
 * The project's files, from file-tree/components/file-tree-root.
 *
 * The providers are the structure here: what is selected, what is being
 * created or renamed, what is being dragged, and where the context menu is.
 * Keeping them apart means a rename does not re-render the whole tree and a
 * drag does not touch the selection until it is dropped.
 *
 * The tree itself is built from the flat list of entries the API sends, which
 * is this client's shape for a project; the nesting is rebuilt in tree.ts.
 */

import { memo, useMemo, type ReactNode } from 'react'
import cx from '@/lib/cx'
import { withErrorBoundary, ErrorBoundaryFallback } from '@/components/ol/error-boundary'
import { useProject } from '@/features/ide/contexts/project-context'
import { useConnection } from '@/features/ide/contexts/connection-context'
import { FileTreeMainProvider } from './contexts/file-tree-main'
import { FileTreeSelectableProvider, useFileTreeSelectable } from './contexts/file-tree-selectable'
import { FileTreeActionableProvider } from './contexts/file-tree-actionable'
import { FileTreeDraggableProvider, useDroppable, useFileTreeDraggable } from './contexts/file-tree-draggable'
import FileTreeFolderList from './components/file-tree-folder-list'
import FileTreeContextMenu from './components/file-tree-context-menu'
import FileTreeModalDelete from './components/modals/file-tree-modal-delete'
import FileTreeModalCreateFile from './components/modals/file-tree-modal-create-file'
import FileTreeModalCreateFolder from './components/modals/file-tree-modal-create-folder'
import FileTreeModalError from './components/modals/file-tree-modal-error'
import { buildTree } from './tree'

/** Every provider the tree needs, in one place so the order is stated once. */
export function FileTreeProviders({ children }: { children: ReactNode }) {
  return (
    <FileTreeMainProvider>
      <FileTreeSelectableProvider>
        <FileTreeActionableProvider>
          <FileTreeDraggableProvider>{children}</FileTreeDraggableProvider>
        </FileTreeActionableProvider>
      </FileTreeSelectableProvider>
    </FileTreeMainProvider>
  )
}

function FileTreeInner({ children }: { children: ReactNode }) {
  const { setIsRootFolderSelected, selectedEntityIds, select } = useFileTreeSelectable()

  // A click on the empty space below the rows means "nothing in particular",
  // which is how somebody drops a multiple selection they no longer want.
  const handleFileTreeClick = () => {
    setIsRootFolderSelected(true)
    if (selectedEntityIds.size > 1) {
      select([])
    }
  }

  return (
    <div className="file-tree-inner" onClick={handleFileTreeClick} data-testid="file-tree-inner">
      {children}
    </div>
  )
}

function FileTreeRootFolder() {
  const { files } = useProject()
  const { isDragging } = useFileTreeDraggable()
  const { isOver, dropProps } = useDroppable(undefined)

  const nodes = useMemo(() => buildTree(files), [files])

  return (
    <FileTreeFolderList
      nodes={nodes}
      className={cx('file-tree-list', {
        'file-tree-dragging': isDragging,
        'dnd-droppable-hover': isOver,
      })}
      dropProps={dropProps}
      dataTestId="file-tree-list-root"
    />
  )
}

function FileTreeBody() {
  const { state } = useConnection()
  const connected = state === 'connected'

  return (
    <>
      {connected ? null : <div className="disconnected-overlay" />}
      <FileTreeContextMenu />
      <FileTreeInner>
        <FileTreeRootFolder />
      </FileTreeInner>
      <FileTreeModalDelete />
      <FileTreeModalCreateFile />
      <FileTreeModalCreateFolder />
      <FileTreeModalError />
    </>
  )
}

export const FileTree = memo(
  withErrorBoundary(FileTreeBody, () => <ErrorBoundaryFallback />)
)

export default FileTree
