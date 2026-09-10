'use client'

/**
 * Creating, renaming and deleting, from
 * file-tree/contexts/file-tree-actionable.
 *
 * One state machine rather than a flag per dialog, because these are mutually
 * exclusive: starting a rename while a delete is being confirmed would leave
 * two dialogs open about the same file. Every start resets the rest.
 *
 * Names are checked here before anything is sent. A duplicate or an unusable
 * name should be refused next to the input that holds it, not as a failed
 * request a moment later.
 */

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useReducer,
  useState,
  type ReactNode,
} from 'react'
import type { FileEntry } from '@/lib/editor'
import { createDocument, createFolder, deleteEntry, renameEntry, uploadFile } from '@/lib/editor'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useFileTreeSelectable } from './file-tree-selectable'
import { isBlockedFilename, isCleanFilename } from '../util/safe-path'
import {
  BlockedFilenameError,
  DuplicateFilenameError,
  InvalidFilenameError,
} from '../errors'

export type NewFileCreateMode = 'doc' | 'upload' | 'project' | 'url'

type State = {
  isDeleting: boolean
  isRenaming: boolean
  isCreatingFile: boolean
  isCreatingFolder: boolean
  isMoving: boolean
  inFlight: boolean
  actionedEntities: FileEntry[] | null
  newFileCreateMode: NewFileCreateMode | null
  error: unknown | null
}

const defaultState: State = {
  isDeleting: false,
  isRenaming: false,
  isCreatingFile: false,
  isCreatingFolder: false,
  isMoving: false,
  inFlight: false,
  actionedEntities: null,
  newFileCreateMode: null,
  error: null,
}

type Action =
  | { type: 'START_RENAME' }
  | { type: 'START_DELETE'; actionedEntities: FileEntry[] | null }
  | { type: 'START_CREATE_FILE'; newFileCreateMode: NewFileCreateMode | null }
  | { type: 'START_CREATE_FOLDER' }
  | { type: 'CREATING_FILE' }
  | { type: 'CREATING_FOLDER' }
  | { type: 'DELETING' }
  | { type: 'MOVING' }
  | { type: 'CLEAR' }
  | { type: 'CANCEL' }
  | { type: 'ERROR'; error: unknown }

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case 'START_RENAME':
      return { ...defaultState, isRenaming: true }
    case 'START_DELETE':
      return { ...defaultState, isDeleting: true, actionedEntities: action.actionedEntities }
    case 'START_CREATE_FILE':
      return { ...defaultState, isCreatingFile: true, newFileCreateMode: action.newFileCreateMode }
    case 'START_CREATE_FOLDER':
      return { ...defaultState, isCreatingFolder: true }
    case 'CREATING_FILE':
      return {
        ...defaultState,
        isCreatingFile: true,
        newFileCreateMode: state.newFileCreateMode,
        inFlight: true,
      }
    case 'CREATING_FOLDER':
      return { ...defaultState, isCreatingFolder: true, inFlight: true }
    case 'DELETING':
      // The list of what is being deleted stays, so the dialog can go on
      // naming the files while the requests are in flight.
      return {
        ...defaultState,
        isDeleting: true,
        inFlight: true,
        actionedEntities: state.actionedEntities,
      }
    case 'MOVING':
      return { ...defaultState, isMoving: true, inFlight: true }
    case 'CLEAR':
      return { ...defaultState }
    case 'CANCEL':
      // A request already sent cannot be called back by closing the dialog.
      return state.inFlight ? state : { ...defaultState }
    case 'ERROR':
      return { ...state, inFlight: false, error: action.error }
    default:
      return state
  }
}

function readOnlyReducer(state: State): State {
  return state
}

type ActionableValue = State & {
  canDelete: boolean
  canBulkDelete: boolean
  canRename: boolean
  canCreate: boolean
  canSetRootDocId: boolean
  parentFolderId: string | undefined
  selectedFileName: string | undefined
  downloadPath: string | undefined
  isDuplicate: (parentFolderId: string | undefined, name: string) => boolean
  startRenaming: () => void
  finishRenaming: (name: string) => Promise<void>
  startDeleting: () => void
  finishDeleting: () => Promise<void>
  startCreatingFile: (mode: NewFileCreateMode) => void
  startCreatingFolder: () => void
  finishCreatingFolder: (name: string) => Promise<void>
  startCreatingDocOrFile: () => void
  startUploadingDocOrFile: () => void
  finishCreatingDoc: (input: { name: string; content?: string }) => Promise<void>
  finishUploadingFiles: (files: File[]) => Promise<void>
  setRootDocId: () => Promise<void>
  cancel: () => void
}

const FileTreeActionableContext = createContext<ActionableValue | undefined>(undefined)

export function FileTreeActionableProvider({ children }: { children: ReactNode }) {
  const { projectId, project, files, canWrite, refresh, chooseRootDoc } = useProject()
  const editor = useEditor()
  const { selectedEntityIds, selectedEntities, select } = useFileTreeSelectable()

  const [state, dispatch] = useReducer(canWrite ? reducer : readOnlyReducer, defaultState)
  const [uploadFolderId, setUploadFolderId] = useState<string | undefined>()

  const selectedCount = selectedEntityIds.size
  const firstSelected = selectedEntities[0]

  // Where a new thing goes: inside the selected folder, or beside the
  // selected file. One selection, or the question has no single answer.
  const parentFolderId = useMemo(() => {
    if (selectedCount !== 1 || !firstSelected) {
      return undefined
    }
    if (firstSelected.kind === 'folder') {
      return firstSelected.id
    }
    return firstSelected.parentId
  }, [selectedCount, firstSelected])

  const namesIn = useCallback(
    (folderId: string | undefined) => {
      const names = new Set<string>()
      for (const entry of files) {
        if ((entry.parentId ?? undefined) === (folderId ?? undefined)) {
          names.add(entry.name)
        }
      }
      return names
    },
    [files]
  )

  const isDuplicate = useCallback(
    (folderId: string | undefined, name: string) => namesIn(folderId).has(name),
    [namesIn]
  )

  const canRename = canWrite && selectedCount === 1 && Boolean(firstSelected)
  const canDelete = canWrite && selectedCount === 1 && Boolean(firstSelected)
  const canBulkDelete = canWrite && selectedCount > 1
  const canCreate = canWrite
  const canSetRootDocId =
    canWrite &&
    selectedCount === 1 &&
    firstSelected?.kind === 'doc' &&
    firstSelected.id !== project.rootDocId &&
    /\.tex$/i.test(firstSelected.name)

  const downloadPath = useMemo(() => {
    if (selectedCount !== 1 || !firstSelected || firstSelected.kind === 'folder') {
      return undefined
    }
    return `/api/projects/${projectId}/files/${firstSelected.id}`
  }, [selectedCount, firstSelected, projectId])

  const cancel = useCallback(() => dispatch({ type: 'CANCEL' }), [])

  /** The checks a name has to pass before anything is sent. */
  const validate = useCallback(
    (name: string, folderId: string | undefined, currentName?: string) => {
      if (!isCleanFilename(name)) {
        throw new InvalidFilenameError()
      }
      if (isBlockedFilename(name)) {
        throw new BlockedFilenameError()
      }
      if (name !== currentName && isDuplicate(folderId, name)) {
        throw new DuplicateFilenameError()
      }
    },
    [isDuplicate]
  )

  const startRenaming = useCallback(() => dispatch({ type: 'START_RENAME' }), [])

  const finishRenaming = useCallback(
    async (name: string) => {
      const entry = firstSelected
      if (!entry || name === entry.name) {
        dispatch({ type: 'CLEAR' })
        return
      }
      try {
        validate(name, entry.parentId, entry.name)
      } catch (error) {
        dispatch({ type: 'ERROR', error })
        return
      }
      try {
        await renameEntry(projectId, entry.id, name)
        await refresh()
        dispatch({ type: 'CLEAR' })
      } catch (error) {
        dispatch({ type: 'ERROR', error })
      }
    },
    [firstSelected, projectId, refresh, validate]
  )

  const startDeleting = useCallback(() => {
    dispatch({ type: 'START_DELETE', actionedEntities: selectedEntities })
  }, [selectedEntities])

  const finishDeleting = useCallback(async () => {
    dispatch({ type: 'DELETING' })
    try {
      // One at a time rather than all at once: the server renumbers the tree
      // on each delete, and a batch that raced with itself would sometimes
      // delete the wrong thing.
      for (const entity of state.actionedEntities ?? []) {
        await deleteEntry(projectId, entity.id)
      }
      await refresh()
      select([])
      dispatch({ type: 'CLEAR' })
    } catch (error) {
      dispatch({ type: 'ERROR', error })
    }
  }, [state.actionedEntities, projectId, refresh, select])

  const startCreatingFile = useCallback((mode: NewFileCreateMode) => {
    dispatch({ type: 'START_CREATE_FILE', newFileCreateMode: mode })
  }, [])

  const startCreatingDocOrFile = useCallback(() => {
    dispatch({ type: 'START_CREATE_FILE', newFileCreateMode: 'doc' })
  }, [])

  const startUploadingDocOrFile = useCallback(() => {
    dispatch({ type: 'START_CREATE_FILE', newFileCreateMode: 'upload' })
  }, [])

  const startCreatingFolder = useCallback(() => {
    dispatch({ type: 'START_CREATE_FOLDER' })
  }, [])

  const finishCreatingFolder = useCallback(
    async (name: string) => {
      try {
        validate(name, parentFolderId)
      } catch (error) {
        dispatch({ type: 'ERROR', error })
        return
      }
      dispatch({ type: 'CREATING_FOLDER' })
      try {
        await createFolder(projectId, { name, folderId: parentFolderId })
        await refresh()
        dispatch({ type: 'CLEAR' })
      } catch (error) {
        dispatch({ type: 'ERROR', error })
      }
    },
    [parentFolderId, projectId, refresh, validate]
  )

  const finishCreatingDoc = useCallback(
    async ({ name, content }: { name: string; content?: string }) => {
      try {
        validate(name, parentFolderId)
      } catch (error) {
        dispatch({ type: 'ERROR', error })
        return
      }
      dispatch({ type: 'CREATING_FILE' })
      try {
        const created = await createDocument(projectId, { name, folderId: parentFolderId, content })
        await refresh()
        dispatch({ type: 'CLEAR' })
        if (created) {
          select(created.id)
          editor.open(created)
        }
      } catch (error) {
        dispatch({ type: 'ERROR', error })
      }
    },
    [parentFolderId, projectId, refresh, validate, select, editor]
  )

  const finishUploadingFiles = useCallback(
    async (uploads: File[]) => {
      dispatch({ type: 'CREATING_FILE' })
      const folderId = uploadFolderId ?? parentFolderId
      try {
        for (const file of uploads) {
          await uploadFile(projectId, file, folderId)
        }
        await refresh()
        dispatch({ type: 'CLEAR' })
      } catch (error) {
        dispatch({ type: 'ERROR', error })
      } finally {
        setUploadFolderId(undefined)
      }
    },
    [projectId, refresh, uploadFolderId, parentFolderId]
  )

  const setRootDocId = useCallback(async () => {
    if (firstSelected?.kind === 'doc') {
      await chooseRootDoc(firstSelected.id)
    }
  }, [firstSelected, chooseRootDoc])

  const value = useMemo<ActionableValue>(
    () => ({
      ...state,
      canDelete,
      canBulkDelete,
      canRename,
      canCreate,
      canSetRootDocId,
      parentFolderId,
      selectedFileName: firstSelected?.name,
      downloadPath,
      isDuplicate,
      startRenaming,
      finishRenaming,
      startDeleting,
      finishDeleting,
      startCreatingFile,
      startCreatingFolder,
      finishCreatingFolder,
      startCreatingDocOrFile,
      startUploadingDocOrFile,
      finishCreatingDoc,
      finishUploadingFiles,
      setRootDocId,
      cancel,
    }),
    [
      state,
      canDelete,
      canBulkDelete,
      canRename,
      canCreate,
      canSetRootDocId,
      parentFolderId,
      firstSelected,
      downloadPath,
      isDuplicate,
      startRenaming,
      finishRenaming,
      startDeleting,
      finishDeleting,
      startCreatingFile,
      startCreatingFolder,
      finishCreatingFolder,
      startCreatingDocOrFile,
      startUploadingDocOrFile,
      finishCreatingDoc,
      finishUploadingFiles,
      setRootDocId,
      cancel,
    ]
  )

  return <FileTreeActionableContext.Provider value={value}>{children}</FileTreeActionableContext.Provider>
}

export function useFileTreeActionable(): ActionableValue {
  const context = useContext(FileTreeActionableContext)
  if (!context) {
    throw new Error('useFileTreeActionable is only available inside FileTreeActionableProvider')
  }
  return context
}
