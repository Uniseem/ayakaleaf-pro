'use client'

/**
 * Searching every file in the project.
 *
 * The search runs on the server: the browser holds only the documents that
 * have been opened, so searching what it has would quietly miss most of the
 * project -- which is worse than not offering search at all, because the
 * empty result looks like an answer.
 */

import { Button, Checkbox, Input, ScrollShadow, Spinner } from '@heroui/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { searchProject, type SearchHit } from '@/lib/search'
import { messageFor } from '@/lib/api'
import { useTranslation } from '@/lib/i18n'
import { RailPanelHeader } from '@/features/ide/components/rail/rail-parts'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'

export function ProjectSearch() {
  const { projectId, entryById } = useProject()
  const editor = useEditor()

  const [query, setQuery] = useState('')
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [wholeWord, setWholeWord] = useState(false)
  const [hits, setHits] = useState<SearchHit[] | null>(null)
  const [searching, setSearching] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // The request in flight, so a slower earlier one cannot overwrite a newer
  // result -- which is the classic search bug.
  const running = useRef<AbortController | null>(null)

  const run = useCallback(
    async (text: string) => {
      running.current?.abort()
      if (!text.trim()) {
        setHits(null)
        setError(null)
        return
      }
      const controller = new AbortController()
      running.current = controller
      setSearching(true)
      setError(null)
      try {
        const found = await searchProject(
          projectId,
          { query: text, caseSensitive, wholeWord },
          controller.signal
        )
        if (!controller.signal.aborted) {
          setHits(found)
        }
      } catch (thrown) {
        if (!controller.signal.aborted) {
          setError(messageFor(thrown))
        }
      } finally {
        if (running.current === controller) {
          running.current = null
          setSearching(false)
        }
      }
    },
    [projectId, caseSensitive, wholeWord]
  )

  // Search after a pause, not on every keystroke.
  useEffect(() => {
    const timer = setTimeout(() => void run(query), 300)
    return () => clearTimeout(timer)
  }, [query, run])

  useEffect(() => () => running.current?.abort(), [])

  const total = hits?.reduce((sum, file) => sum + file.matches.length, 0) ?? 0

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="border-b border-divider px-3 py-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-default-500">
          Search
        </span>
      </header>

      <div className="flex flex-col gap-2 border-b border-divider p-2">
        <Input
          autoFocus
          size="sm"
          value={query}
          onValueChange={setQuery}
          placeholder="Find in project"
          isClearable
          onClear={() => setQuery('')}
        />
        <div className="flex gap-3">
          <Checkbox
            size="sm"
            isSelected={caseSensitive}
            onValueChange={setCaseSensitive}
            classNames={{ label: 'text-xs' }}
          >
            Match case
          </Checkbox>
          <Checkbox
            size="sm"
            isSelected={wholeWord}
            onValueChange={setWholeWord}
            classNames={{ label: 'text-xs' }}
          >
            Whole word
          </Checkbox>
        </div>
      </div>

      <ScrollShadow className="min-h-0 flex-1">
        {searching ? (
          <div className="flex justify-center py-6">
            <Spinner size="sm" />
          </div>
        ) : error ? (
          <p className="p-3 text-xs text-danger">{error}</p>
        ) : hits === null ? (
          <p className="p-3 text-xs text-default-400">
            Type to search every file in this project.
          </p>
        ) : total === 0 ? (
          <p className="p-3 text-xs text-default-400">Nothing found.</p>
        ) : (
          <div className="p-1">
            <p className="px-2 py-1 text-[11px] text-default-500">
              {total} result{total === 1 ? '' : 's'} in {hits.length} file
              {hits.length === 1 ? '' : 's'}
            </p>
            {hits.map(file => (
              <section key={file.path} className="mb-1">
                <p className="truncate px-2 py-1 text-xs font-medium" title={file.path}>
                  {file.path}
                </p>
                <ul>
                  {file.matches.map((match, index) => (
                    <li key={`${match.line}-${index}`}>
                      <button
                        type="button"
                        className="block w-full truncate rounded px-2 py-0.5 text-left font-mono text-[11px] text-default-600 hover:bg-default-100"
                        onClick={() => {
                          const entry = entryById(file.id)
                          if (entry) {
                            editor.open(entry)
                            window.dispatchEvent(
                              new CustomEvent('ide:goto-line', {
                                detail: { line: match.line },
                              })
                            )
                          }
                        }}
                      >
                        <span className="mr-2 text-default-400">
                          {match.line + 1}
                        </span>
                        {match.text.trim()}
                      </button>
                    </li>
                  ))}
                </ul>
              </section>
            ))}
          </div>
        )}
      </ScrollShadow>

      {hits !== null ? (
        <div className="border-t border-divider p-2">
          <Button
            size="sm"
            variant="light"
            className="h-7 w-full text-xs"
            onPress={() => {
              setQuery('')
              setHits(null)
            }}
          >
            Clear
          </Button>
        </div>
      ) : null}
    </div>
  )
}

/** The search as the rail shows it: under the panel's header. */
export function ProjectSearchPanel() {
  const { t } = useTranslation()
  return (
    <div className="flex h-full min-h-0 flex-col">
      <RailPanelHeader title={t('project_search')} />
      <div className="min-h-0 flex-1">
        <ProjectSearch />
      </div>
    </div>
  )
}
