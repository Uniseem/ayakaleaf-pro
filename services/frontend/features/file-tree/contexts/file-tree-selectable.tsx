'use client'

/**
 * What is selected in the tree, from file-tree/contexts/file-tree-selectable.
 *
 * A set rather than one id, because deleting six files should be one action
 * rather than six. Plain click replaces the selection, Ctrl or Cmd adds to it,
 * and Shift takes everything between the last selection and the click -- over
 * the tree in the order it is drawn, so a range means what it looks like.
 *
 * Selecting is also opening: a document goes to the editor and a binary file
 * to the viewer, which is why this ends up calling into the editor context.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useState,
  type ReactNode,
} from 'react'
import cx from '@/lib/cx'
import { isMac, useEventListener } from '@/lib/hooks'
import type { FileEntry } from '@/lib/editor'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useFileTreeMainContext } from './file-tree-main'
import { buildTree, type TreeNode } from '../tree'

type SelectableValue = {
  selectedEntityIds: Set<string>
  selectedEntities: FileEntry[]
  isRootFolderSelected: boolean
  setIsRootFolderSelected: (value: boolean) => void
  selectedEntityParentIds: Set<string>
  select: (id: string | string[]) => void
  unselect: (id: string) => void
  selectOrMultiSelectEntity: (id: string | string[], multiple?: boolean) => void
}

const FileTreeSelectableContext = createContext<SelectableValue | undefined>(undefined)

type Action =
  | { type: 'SELECT'; id: string | string[] }
  | { type: 'MULTI_SELECT'; id: string }
  | { type: 'UNSELECT'; id: string }

function readWriteReducer(selected: Set<string>, action: Action): Set<string> {
  switch (action.type) {
    case 'SELECT':
      return new Set(Array.isArray(action.id) ? action.id : [action.id])

    case 'MULTI_SELECT': {
      const next = new Set(selected)
      if (next.has(action.id)) {
        // Already in the selection. Clicking it again takes it out, unless it
        // is the only one: a selection of nothing has no meaning here.
        if (next.size > 1) {
          next.delete(action.id)
        }
      } else {
        next.add(action.id)
      }
      return next
    }

    case 'UNSELECT': {
      const next = new Set(selected)
      next.delete(action.id)
      return next
    }

    default:
      return selected
  }
}

function readOnlyReducer(selected: Set<string>, action: Action): Set<string> {
  if (action.type === 'SELECT') {
    return new Set(Array.isArray(action.id) ? action.id : [action.id])
  }
  // Nothing to do in bulk without write access, so multi-select is not offered.
  return selected
}

/** Every entry in the order the tree draws it, which is what a range means. */
function* sortedIds(nodes: TreeNode[]): Generator<string> {
  for (const node of nodes) {
    yield node.entry.id
    if (node.children.length) {
      yield* sortedIds(node.children)
    }
  }
}

export function FileTreeSelectableProvider({ children }: { children: ReactNode }) {
  const { files, canWrite, entryById } = useProject()
  const editor = useEditor()

  const readOnly = !canWrite

  const [isRootFolderSelected, setIsRootFolderSelected] = useState(false)

  const [selectedEntityIds, dispatch] = useReducer(
    readOnly ? readOnlyReducer : readWriteReducer,
    null,
    () => {
      const open = editor.current?.id ?? editor.currentFile?.id
      return open ? new Set([open]) : new Set<string>()
    }
  )

  const selectedEntities = useMemo(
    () =>
      Array.from(selectedEntityIds)
        .map(id => entryById(id))
        .filter((entry): entry is FileEntry => Boolean(entry)),
    [selectedEntityIds, entryById]
  )

  // Every folder on the way to something selected, so those folders can open
  // themselves and the selection is not hidden inside a closed one.
  const selectedEntityParentIds = useMemo(() => {
    const ids = new Set<string>()
    for (const entry of selectedEntities) {
      const parts = entry.path.split('/')
      parts.pop()
      let prefix = ''
      for (const part of parts) {
        prefix = prefix ? `${prefix}/${part}` : part
        const folder = files.find(candidate => candidate.path === prefix)
        if (folder) {
          ids.add(folder.id)
        }
      }
    }
    return ids
  }, [selectedEntities, files])

  // Opening a file from anywhere else -- the outline, a log entry, a search
  // result -- should move the selection here too, or the tree would be showing
  // something other than what is open.
  useEventListener(
    'entity:opened' as keyof WindowEventMap,
    useCallback((event: Event) => {
      const id = (event as CustomEvent<string>).detail
      if (id) {
        dispatch({ type: 'SELECT', id })
      }
    }, [])
  )

  const openId = editor.current?.id ?? editor.currentFile?.id
  useEffect(() => {
    if (openId) {
      dispatch({ type: 'SELECT', id: openId })
    }
  }, [openId])

  const select = useCallback((id: string | string[]) => {
    dispatch({ type: 'SELECT', id })
  }, [])

  const unselect = useCallback((id: string) => {
    dispatch({ type: 'UNSELECT', id })
  }, [])

  const selectOrMultiSelectEntity = useCallback((id: string | string[], multiple?: boolean) => {
    if (multiple && !Array.isArray(id)) {
      dispatch({ type: 'MULTI_SELECT', id })
    } else {
      dispatch({ type: 'SELECT', id })
    }
  }, [])

  const value = useMemo<SelectableValue>(
    () => ({
      selectedEntityIds,
      selectedEntities,
      selectedEntityParentIds,
      select,
      unselect,
      selectOrMultiSelectEntity,
      isRootFolderSelected,
      setIsRootFolderSelected,
    }),
    [
      selectedEntityIds,
      selectedEntities,
      selectedEntityParentIds,
      select,
      unselect,
      selectOrMultiSelectEntity,
      isRootFolderSelected,
    ]
  )

  return <FileTreeSelectableContext.Provider value={value}>{children}</FileTreeSelectableContext.Provider>
}

export function useFileTreeSelectable(): SelectableValue {
  const context = useContext(FileTreeSelectableContext)
  if (!context) {
    throw new Error('useFileTreeSelectable is only available inside FileTreeSelectableProvider')
  }
  return context
}

/** The props one row needs to take part in the selection. */
export function useSelectableEntity(id: string, type: 'doc' | 'file' | 'folder') {
  const { files, entryById } = useProject()
  const editor = useEditor()
  const { view, setView } = useLayout()
  const { setContextMenuCoords } = useFileTreeMainContext()
  const {
    selectedEntityIds,
    selectOrMultiSelectEntity,
    isRootFolderSelected,
    setIsRootFolderSelected,
  } = useFileTreeSelectable()

  const isSelected = selectedEntityIds.has(id)

  const buildSelectedRange = useCallback(
    (target: string) => {
      const order = Array.from(sortedIds(buildTree(files)))
      const selected: string[] = []
      let started = false

      for (const itemId of order) {
        if (itemId === target) {
          selected.push(itemId)
          if (started) {
            break
          }
          started = true
        } else if (selectedEntityIds.has(itemId)) {
          selected.push(itemId)
          if (started) {
            break
          }
          started = true
        } else if (started) {
          selected.push(itemId)
        }
      }

      return selected
    },
    [files, selectedEntityIds]
  )

  const handleEvent = useCallback(
    (event: React.MouseEvent | React.KeyboardEvent) => {
      event.stopPropagation()
      // Command on a Mac, Control everywhere else. Not offered while the root
      // folder is what is selected, because that is a different kind of thing.
      const multiSelect = !isRootFolderSelected && (isMac() ? event.metaKey : event.ctrlKey)
      setIsRootFolderSelected(false)

      if (event.shiftKey) {
        selectOrMultiSelectEntity(buildSelectedRange(id))
      } else {
        selectOrMultiSelectEntity(id, multiSelect)
      }

      // Selecting is opening. A folder keeps whichever pane is showing.
      if (type !== 'folder' && !multiSelect && !event.shiftKey) {
        const entry = entryById(id)
        if (entry) {
          editor.open(entry)
        }
        setView(type === 'file' ? 'file' : 'editor')
      }
    },
    [
      id,
      type,
      isRootFolderSelected,
      setIsRootFolderSelected,
      selectOrMultiSelectEntity,
      buildSelectedRange,
      entryById,
      editor,
      setView,
    ]
  )

  const handleClick = useCallback(
    (event: React.MouseEvent) => {
      handleEvent(event)
      if (!event.ctrlKey && !event.metaKey) {
        setContextMenuCoords(null)
      }
    },
    [handleEvent, setContextMenuCoords]
  )

  const handleKeyPress = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter' || event.key === ' ') {
        handleEvent(event)
      }
    },
    [handleEvent]
  )

  const handleContextMenu = useCallback(
    (event: React.MouseEvent) => {
      // Right-clicking something outside the selection selects it first, so
      // the menu is never about a row other than the one under the pointer.
      if (!selectedEntityIds.has(id)) {
        handleEvent(event)
      }
    },
    [id, handleEvent, selectedEntityIds]
  )

  const isVisuallySelected = !isRootFolderSelected && isSelected && view !== 'pdf'

  const props = useMemo(
    () => ({
      className: cx({ selected: isVisuallySelected }),
      'aria-selected': isVisuallySelected,
      onClick: handleClick,
      onContextMenu: handleContextMenu,
      onKeyPress: handleKeyPress,
    }),
    [handleClick, handleContextMenu, handleKeyPress, isVisuallySelected]
  )

  return { isSelected, props }
}
