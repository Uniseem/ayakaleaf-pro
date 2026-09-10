'use client'

/**
 * Searching every file in the project.
 *
 * The search runs on the server: the browser holds only the documents that
 * have been opened, so searching what it has would quietly miss most of the
 * project -- which is worse than not offering search at all, because the
 * empty result looks like an answer.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import cx from '@/lib/cx'
import { searchProject, type SearchHit } from '@/lib/search'
import { messageFor } from '@/lib/api'
import { useTranslation } from '@/lib/i18n'
import { Button } from '@/components/ol/button'
import { OLFormControl } from '@/components/ol/form-control'
import { Spinner } from '@/components/ol/spinner'
import MaterialIcon from '@/components/ol/material-icon'
import { RailPanelHeader } from '@/features/ide/components/rail/rail-parts'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'

export function ProjectSearch() {
  const { t } = useTranslation()
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

  // After a pause, not on every keystroke.
  useEffect(() => {
    const timer = setTimeout(() => void run(query), 300)
    return () => clearTimeout(timer)
  }, [query, run])

  useEffect(() => () => running.current?.abort(), [])

  const total = hits?.reduce((sum, file) => sum + file.matches.length, 0) ?? 0

  return (
    <div className="project-search">
      <div className="project-search-form">
        <div className="project-search-input">
          <MaterialIcon type="search" className="project-search-input-icon" />
          <OLFormControl
            autoFocus
            type="text"
            value={query}
            onChange={event => setQuery(event.target.value)}
            placeholder={t('search')}
          />
          {query && (
            <button
              type="button"
              className="project-search-clear"
              aria-label={t('clear_search')}
              onClick={() => setQuery('')}
            >
              <MaterialIcon type="close" />
            </button>
          )}
        </div>
        <div className="project-search-options">
          <label className="project-search-option">
            <input
              type="checkbox"
              checked={caseSensitive}
              onChange={event => setCaseSensitive(event.target.checked)}
            />
            {t('search_match_case')}
          </label>
          <label className="project-search-option">
            <input
              type="checkbox"
              checked={wholeWord}
              onChange={event => setWholeWord(event.target.checked)}
            />
            {t('search_whole_word')}
          </label>
        </div>
      </div>

      <div className="project-search-results">
        {searching ? (
          <div className="project-search-loading">
            <Spinner size="sm" />
          </div>
        ) : error ? (
          <p className="project-search-message project-search-message-error">{error}</p>
        ) : hits === null ? (
          <p className="project-search-message">{t('search')}</p>
        ) : total === 0 ? (
          <p className="project-search-message">{t('no_search_results')}</p>
        ) : (
          <>
            <p className="project-search-count">
              {t('project_search_result_count', { count: total })}
            </p>
            {hits.map(file => (
              <section key={file.path} className="project-search-file">
                <p className="project-search-file-name" title={file.path}>
                  {file.path}
                </p>
                <ul className="list-unstyled">
                  {file.matches.map((match, index) => (
                    <li key={`${match.line}-${index}`}>
                      <button
                        type="button"
                        className="project-search-match"
                        onClick={() => {
                          const entry = entryById(file.id)
                          if (entry) {
                            editor.open(entry)
                            window.dispatchEvent(
                              new CustomEvent('ide:goto-line', { detail: { line: match.line } })
                            )
                          }
                        }}
                      >
                        <span className="project-search-match-line">{match.line + 1}</span>
                        <span className="project-search-match-text">{match.text.trim()}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              </section>
            ))}
          </>
        )}
      </div>

      {hits !== null ? (
        <div className="project-search-footer">
          <Button
            size="sm"
            variant="secondary"
            className={cx('project-search-clear-button')}
            onClick={() => {
              setQuery('')
              setHits(null)
            }}
          >
            {t('clear_search')}
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
    <div className="project-search-panel">
      <RailPanelHeader title={t('project_search')} />
      <ProjectSearch />
    </div>
  )
}

export default ProjectSearch
