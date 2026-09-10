import { useCallback, useMemo } from 'react'
import MiniSearch from 'minisearch'
import { CommandPaletteSearchResult, CommandPaletteSource } from '../types'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { debugConsole } from '@/lib/debug'

/**
 * The project's files, as something the palette can search.
 *
 * The index is built from the flat entry list the project context already
 * holds -- there is no tree to walk here, because paths are on the entries.
 * Both the name and the path are indexed, so "chapters/intro" finds the file
 * as readily as "intro" does.
 */
const useFileTreeCommandSource = (): CommandPaletteSource => {
  const { files, entryById } = useProject()
  const editor = useEditor()

  const openable = useMemo(
    () =>
      files
        .filter(entry => entry.kind !== 'folder')
        .map(entry => ({ id: entry.id, name: entry.name, path: entry.path })),
    [files]
  )

  const index = useMemo(() => {
    const miniSearch = new MiniSearch({
      fields: ['path', 'name'],
      storeFields: ['path', 'name'],
    })
    miniSearch.addAll(openable)
    return miniSearch
  }, [openable])

  const onSelect = useCallback(
    (id: string) => {
      const entry = entryById(id)
      if (!entry) {
        debugConsole.error('Attempting to open a file that is not in the project')
        return
      }
      editor.open(entry)
    },
    [entryById, editor]
  )

  const defaults = useCallback(
    (): CommandPaletteSearchResult[] =>
      openable.slice(0, 10).map(({ path, name, id }) => ({
        title: name,
        description: path === name ? undefined : path,
        onSelect: () => onSelect(id),
        score: 1,
      })),
    [openable, onSelect]
  )

  return useMemo<CommandPaletteSource>(
    () => ({
      id: 'file-tree',
      search(query) {
        const result = index.search(query, {
          prefix: true,
          fuzzy: term => (term.length > 3 ? 0.2 : false),
        })
        return result.map(({ path, name, id, score }) => ({
          title: name as string,
          description: path === name ? undefined : (path as string),
          onSelect: () => onSelect(id as string),
          score,
        }))
      },
      defaults,
    }),
    [index, onSelect, defaults]
  )
}

export default useFileTreeCommandSource
