'use client'

/**
 * Dragging files between folders, from
 * file-tree/contexts/file-tree-draggable.
 *
 * The browser's own drag and drop rather than a library: what is being dragged
 * is one project's files onto one project's folders, the drag never leaves the
 * page, and the original's react-dnd setup exists mostly to support a custom
 * preview layer that the ported CSS draws just as well.
 *
 * A folder cannot be dropped into itself or into anything inside it, which is
 * the one rule that stops the tree becoming a cycle.
 */

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import type { FileEntry } from '@/lib/editor'
import { moveEntry } from '@/lib/editor'
import { useProject } from '@/features/ide/contexts/project-context'
import { useFileTreeSelectable } from './file-tree-selectable'

type DraggableValue = {
  draggedEntityIds: Set<string>
  isDragging: boolean
  startDragging: (id: string) => void
  stopDragging: () => void
  dropOn: (folderId: string | undefined) => Promise<void>
  overFolderId: string | null
  setOverFolderId: (id: string | null) => void
}

const FileTreeDraggableContext = createContext<DraggableValue | undefined>(undefined)

/** Whether moving an entry into a folder makes sense. */
function canMoveInto(entry: FileEntry, folder: FileEntry | undefined, files: FileEntry[]): boolean {
  // Already there.
  if ((entry.parentId ?? undefined) === (folder?.id ?? undefined)) {
    return false
  }
  if (!folder) {
    return true
  }
  if (folder.kind !== 'folder') {
    return false
  }
  if (folder.id === entry.id) {
    return false
  }
  // Into its own descendant would detach that whole subtree from the root.
  if (entry.kind === 'folder' && folder.path.startsWith(entry.path + '/')) {
    return false
  }
  // A name already taken in the destination.
  return !files.some(
    candidate =>
      (candidate.parentId ?? undefined) === folder.id && candidate.name === entry.name
  )
}

export function FileTreeDraggableProvider({ children }: { children: ReactNode }) {
  const { projectId, files, entryById, refresh } = useProject()
  const { selectedEntityIds } = useFileTreeSelectable()

  const [draggedEntityIds, setDraggedEntityIds] = useState<Set<string>>(new Set())
  const [overFolderId, setOverFolderId] = useState<string | null>(null)

  const startDragging = useCallback(
    (id: string) => {
      // Dragging one of several selected files takes all of them, which is
      // what the selection was for.
      setDraggedEntityIds(selectedEntityIds.has(id) ? new Set(selectedEntityIds) : new Set([id]))
    },
    [selectedEntityIds]
  )

  const stopDragging = useCallback(() => {
    setDraggedEntityIds(new Set())
    setOverFolderId(null)
  }, [])

  const dropOn = useCallback(
    async (folderId: string | undefined) => {
      const folder = folderId ? entryById(folderId) : undefined
      const moving = Array.from(draggedEntityIds)
        .map(id => entryById(id))
        .filter((entry): entry is FileEntry => Boolean(entry))
        .filter(entry => canMoveInto(entry, folder, files))

      stopDragging()

      if (!moving.length) {
        return
      }
      for (const entry of moving) {
        await moveEntry(projectId, entry.id, folder?.id)
      }
      await refresh()
    },
    [draggedEntityIds, entryById, files, projectId, refresh, stopDragging]
  )

  const value = useMemo<DraggableValue>(
    () => ({
      draggedEntityIds,
      isDragging: draggedEntityIds.size > 0,
      startDragging,
      stopDragging,
      dropOn,
      overFolderId,
      setOverFolderId,
    }),
    [draggedEntityIds, startDragging, stopDragging, dropOn, overFolderId]
  )

  return <FileTreeDraggableContext.Provider value={value}>{children}</FileTreeDraggableContext.Provider>
}

export function useFileTreeDraggable(): DraggableValue {
  const context = useContext(FileTreeDraggableContext)
  if (!context) {
    throw new Error('useFileTreeDraggable is only available inside FileTreeDraggableProvider')
  }
  return context
}

/** The props a row needs to be picked up. */
export function useDraggable(id: string) {
  const { startDragging, stopDragging, draggedEntityIds } = useFileTreeDraggable()
  const [isDraggable, setIsDraggable] = useState(true)

  const dragProps = useMemo(
    () => ({
      draggable: isDraggable,
      onDragStart: (event: React.DragEvent) => {
        startDragging(id)
        event.dataTransfer.effectAllowed = 'move'
        // Set something, or Firefox will not start the drag at all.
        event.dataTransfer.setData('application/x-ayakaleaf-file-id', id)
      },
      onDragEnd: () => stopDragging(),
    }),
    [id, isDraggable, startDragging, stopDragging]
  )

  return { dragProps, setIsDraggable, isBeingDragged: draggedEntityIds.has(id) }
}

/** The props a folder needs to be dropped onto. */
export function useDroppable(folderId: string | undefined) {
  const { dropOn, overFolderId, setOverFolderId, isDragging } = useFileTreeDraggable()
  const key = folderId ?? '__root__'

  const dropProps = useMemo(
    () => ({
      onDragOver: (event: React.DragEvent) => {
        if (!isDragging) {
          return
        }
        event.preventDefault()
        event.dataTransfer.dropEffect = 'move'
        setOverFolderId(key)
      },
      onDragLeave: (event: React.DragEvent) => {
        // Only when the pointer has left this element rather than crossed into
        // one of its own children, which fires the same event.
        if (event.currentTarget.contains(event.relatedTarget as Node)) {
          return
        }
        setOverFolderId(null)
      },
      onDrop: (event: React.DragEvent) => {
        event.preventDefault()
        event.stopPropagation()
        void dropOn(folderId)
      },
    }),
    [folderId, key, dropOn, isDragging, setOverFolderId]
  )

  return { isOver: overFolderId === key, dropProps }
}
