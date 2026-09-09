'use client'

/**
 * The project the editor has open.
 *
 * Everything that draws the project reads it from here: the file tree, the
 * toolbar's name, the compiler the settings menu shows, what the person is
 * allowed to change. The page fetched it on the server, so this starts with
 * the real thing rather than with nothing and a spinner.
 *
 * Mutations go through here too, and each one refreshes the tree afterwards
 * rather than patching it by hand. The tree is small, the API is the authority
 * on what a rename did to a path, and guessing would eventually be wrong.
 */

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import {
  createDocument,
  createFolder,
  deleteEntry,
  getProject,
  isEditable,
  renameEntry,
  setRootDoc,
  type FileEntry,
  type Project,
  type ProjectView,
} from '@/lib/editor'
import { canWrite, renameProject, type Access } from '@/lib/projects'

export type ProjectValue = {
  projectId: string
  project: Project
  access: Access
  files: FileEntry[]

  /** Whether this person may change anything. */
  canWrite: boolean
  /** Whether this person may leave comments but not edit. */
  canReview: boolean
  isOwner: boolean

  /** The documents that could be compiled, in tree order. */
  docs: FileEntry[]
  entryById: (id: string) => FileEntry | undefined
  entryByPath: (path: string) => FileEntry | undefined

  refresh: () => Promise<void>
  rename: (entryId: string, name: string) => Promise<void>
  remove: (entryId: string) => Promise<void>
  addDoc: (name: string, folderId?: string) => Promise<FileEntry | undefined>
  addFolder: (name: string, folderId?: string) => Promise<void>
  chooseRootDoc: (docId: string) => Promise<void>
  setName: (name: string) => Promise<void>
}

const ProjectContext = createContext<ProjectValue | undefined>(undefined)

export function ProjectProvider({
  initial,
  children,
}: {
  initial: ProjectView
  children: ReactNode
}) {
  const [view, setView] = useState<ProjectView>(initial)

  const projectId = view.project.id

  const refresh = useCallback(async () => {
    setView(await getProject(projectId))
  }, [projectId])

  const rename = useCallback(
    async (entryId: string, name: string) => {
      await renameEntry(projectId, entryId, name)
      await refresh()
    },
    [projectId, refresh]
  )

  const remove = useCallback(
    async (entryId: string) => {
      await deleteEntry(projectId, entryId)
      await refresh()
    },
    [projectId, refresh]
  )

  const addDoc = useCallback(
    async (name: string, folderId?: string) => {
      const created = await createDocument(projectId, { name, folderId })
      await refresh()
      return created
    },
    [projectId, refresh]
  )

  const addFolder = useCallback(
    async (name: string, folderId?: string) => {
      await createFolder(projectId, { name, folderId })
      await refresh()
    },
    [projectId, refresh]
  )

  const chooseRootDoc = useCallback(
    async (docId: string) => {
      await setRootDoc(projectId, docId)
      await refresh()
    },
    [projectId, refresh]
  )

  const setName = useCallback(
    async (name: string) => {
      await renameProject(projectId, name)
      await refresh()
    },
    [projectId, refresh]
  )

  const value = useMemo<ProjectValue>(() => {
    const byId = new Map(view.files.map(entry => [entry.id, entry]))
    const byPath = new Map(view.files.map(entry => [entry.path, entry]))

    return {
      projectId,
      project: view.project,
      access: view.access,
      files: view.files,
      canWrite: canWrite(view.access),
      canReview: view.access === 'review' || canWrite(view.access),
      isOwner: view.access === 'owner',
      docs: view.files.filter(isEditable),
      entryById: id => byId.get(id),
      entryByPath: path => byPath.get(path),
      refresh,
      rename,
      remove,
      addDoc,
      addFolder,
      chooseRootDoc,
      setName,
    }
  }, [
    projectId,
    view,
    refresh,
    rename,
    remove,
    addDoc,
    addFolder,
    chooseRootDoc,
    setName,
  ])

  return (
    <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
  )
}

export function useProject(): ProjectValue {
  const value = useContext(ProjectContext)
  if (!value) {
    throw new Error('useProject must be used inside a ProjectProvider')
  }
  return value
}
