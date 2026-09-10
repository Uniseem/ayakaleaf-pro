import { useMemo } from 'react'
import { File, FileOrDirectory, filterFolders } from '../utils/file'
import { useProject } from '@/features/ide/contexts/project-context'
import type { FileEntry } from '@/lib/editor'

/**
 * The project's folders, as a tree.
 *
 * The project context holds a flat list with paths on it, which is the right
 * shape for nearly everything; the figure modal is the exception, because it
 * offers a folder to put a new file in and has to know what is already inside
 * each one. The tree is rebuilt from the list rather than kept alongside it,
 * so there is still only one thing to keep up to date.
 */
export const useCurrentProjectFolders: () => {
  folders: File[] | undefined
  rootFile: File
  rootFolder: FileOrDirectory
} = () => {
  const { files } = useProject()

  return useMemo(() => {
    const rootFolder = buildTree(files)
    const rootFile = { ...rootFolder, path: '' }
    const folders = filterFolders(rootFolder)
    return { folders, rootFile, rootFolder }
  }, [files])
}

function buildTree(entries: FileEntry[]): FileOrDirectory {
  const root: FileOrDirectory = {
    id: 'root-folder',
    name: 'rootFolder',
    type: 'folder',
    children: [],
  }

  const byId = new Map<string, FileOrDirectory>([[root.id, root]])

  // Folders first, so a child never arrives before the folder holding it.
  const ordered = [...entries].sort((a, b) => a.path.split('/').length - b.path.split('/').length)

  for (const entry of ordered) {
    const node: FileOrDirectory = {
      id: entry.id,
      name: entry.name,
      type: entry.kind,
      ...(entry.kind === 'folder' ? { children: [] } : {}),
    }
    byId.set(entry.id, node)
    const parent = (entry.parentId && byId.get(entry.parentId)) || root
    parent.children!.push(node)
  }

  return root
}
