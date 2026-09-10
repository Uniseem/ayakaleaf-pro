'use client'

/**
 * What happened to this project, from features/history.
 *
 * The document on the left, the versions on the right. The history service has
 * already grouped thousands of keystrokes into sittings of work, which is what
 * makes the list readable at all -- a list of operations would be a list of
 * individual characters.
 *
 * Restoring writes the old content as a new edit rather than rewinding.
 * Undoing a mistake is another thing that happened, and a history that hides
 * it is one you cannot undo an undo in.
 */

import { useCallback, useEffect, useState } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import { Button } from '@/components/ol/button'
import { Badge } from '@/components/ol/badge'
import { LoadingSpinner } from '@/components/ol/spinner'
import { Notification } from '@/components/ol/notification'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { OLFormControl, OLFormGroup, OLFormLabel, OLFormText } from '@/components/ol/form-control'
import MaterialIcon from '@/components/ol/material-icon'
import {
  authorsOf,
  createLabel,
  getChanges,
  getDiff,
  listUpdates,
  restoreFile,
  type DiffChunk,
  type FileChange,
  type Update,
} from '@/lib/history'
import { messageFor } from '@/lib/api'
import { useProject } from '@/features/ide/contexts/project-context'
import { useLayout } from '@/features/ide/contexts/layout-context'

export function HistoryView() {
  const { t } = useTranslation()
  const { projectId, canWrite } = useProject()
  const layout = useLayout()

  const [updates, setUpdates] = useState<Update[]>([])
  const [nextBefore, setNextBefore] = useState<number>(0)
  const [chosen, setChosen] = useState<Update | null>(null)
  const [changes, setChanges] = useState<FileChange[] | null>(null)
  const [path, setPath] = useState<string | null>(null)
  const [diff, setDiff] = useState<DiffChunk[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [labelling, setLabelling] = useState<Update | null>(null)

  const load = useCallback(
    async (before?: number) => {
      setError(null)
      try {
        const answer = await listUpdates(projectId, before)
        setUpdates(previous => (before ? [...previous, ...answer.updates] : answer.updates))
        setNextBefore(answer.nextBefore)
        if (!before && answer.updates.length > 0) {
          setChosen(answer.updates[0] ?? null)
        }
      } catch (thrown) {
        setError(messageFor(thrown))
      } finally {
        setLoading(false)
      }
    },
    [projectId]
  )

  useEffect(() => {
    void load()
  }, [load])

  // What changed in the chosen version.
  useEffect(() => {
    if (!chosen) {
      return
    }
    let live = true
    setChanges(null)
    setPath(null)
    setDiff(null)
    getChanges(projectId, chosen.fromV, chosen.toV)
      .then(answer => {
        if (live) {
          setChanges(answer.diff)
          const first = answer.diff.find(each => each.operation !== 'removed')
          if (first) {
            setPath(first.pathname)
          }
        }
      })
      .catch(thrown => {
        if (live) {
          setError(messageFor(thrown))
        }
      })
    return () => {
      live = false
    }
  }, [chosen, projectId])

  // The chosen file's diff.
  useEffect(() => {
    if (!chosen || !path) {
      return
    }
    let live = true
    setDiff(null)
    getDiff(projectId, path, chosen.fromV, chosen.toV)
      .then(answer => {
        if (live) {
          setDiff(answer.diff)
        }
      })
      .catch(() => {
        if (live) {
          setDiff([])
        }
      })
    return () => {
      live = false
    }
  }, [chosen, path, projectId])

  return (
    <div className="history-react">
      <div className="doc-panel">
        <div className="history-header toolbar-container">
          <div className="history-toolbar">
            <span className="history-toolbar-title">
              {chosen ? t('history') : t('history')}
            </span>
            <div className="history-toolbar-spacer" />
            {chosen && canWrite ? (
              <>
                <Button variant="link" size="sm" onClick={() => setLabelling(chosen)}>
                  {t('history_label_this_version')}
                </Button>
                {path ? (
                  <Button
                    variant="secondary"
                    size="sm"
                    isLoading={busy}
                    onClick={async () => {
                      setBusy(true)
                      setError(null)
                      try {
                        await restoreFile(projectId, path, chosen.fromV)
                      } catch (thrown) {
                        setError(messageFor(thrown))
                      } finally {
                        setBusy(false)
                      }
                    }}
                  >
                    {t('restore_file')}
                  </Button>
                ) : null}
              </>
            ) : null}
            <Button variant="secondary" size="sm" onClick={layout.restoreView}>
              {t('back_to_editor')}
            </Button>
          </div>
        </div>

        {error ? (
          <div className="history-error">
            <Notification type="error" content={error} />
          </div>
        ) : null}

        <div className="doc-container">
          <div className="history-file-tree">
            {changes === null ? (
              <LoadingSpinner />
            ) : changes.length === 0 ? (
              <p className="history-empty">{t('empty')}</p>
            ) : (
              <ul className="list-unstyled">
                {changes.map(change => (
                  <li key={change.pathname}>
                    <button
                      type="button"
                      onClick={() => setPath(change.pathname)}
                      className={cx('history-file-entry', {
                        'history-file-entry-selected': path === change.pathname,
                      })}
                      title={change.pathname}
                    >
                      <ChangeMark operation={change.operation} />
                      <span className="history-file-entry-name">{change.pathname}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="document-diff-container">
            {diff === null ? (
              <LoadingSpinner />
            ) : diff.length === 0 ? (
              <p className="history-empty">{t('empty')}</p>
            ) : (
              <pre className="history-diff">
                {diff.map((chunk, index) => (
                  <DiffPiece key={index} chunk={chunk} />
                ))}
              </pre>
            )}
          </div>
        </div>
      </div>

      <div className="change-list">
        <div className="history-version-list-container">
          {loading ? (
            <LoadingSpinner />
          ) : updates.length === 0 ? (
            <p className="history-empty">{t('empty')}</p>
          ) : (
            updates.map(update => (
              <div
                key={`${update.fromV}-${update.toV}`}
                className={cx('history-version-details', 'history-version-selectable', {
                  'history-version-selected': chosen?.toV === update.toV,
                })}
                role="button"
                tabIndex={0}
                onClick={() => setChosen(update)}
                onKeyDown={event => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault()
                    setChosen(update)
                  }
                }}
              >
                <div className="history-version-metadata-time">
                  <time>
                    {update.meta.end_ts
                      ? new Date(update.meta.end_ts).toLocaleString()
                      : t('history')}
                  </time>
                </div>
                {update.labels?.length ? (
                  <div className="history-version-labels">
                    {update.labels.map(label => (
                      <Badge key={label.id} bg="secondary" className="history-version-label">
                        <MaterialIcon type="label" /> {label.comment}
                      </Badge>
                    ))}
                  </div>
                ) : null}
                <div className="history-version-metadata-users">{authorsOf(update)}</div>
              </div>
            ))
          )}
        </div>
        {nextBefore ? (
          <Button variant="link" size="sm" onClick={() => void load(nextBefore)}>
            {t('show_more')}
          </Button>
        ) : null}
      </div>

      {labelling ? (
        <LabelDialog
          version={labelling.toV}
          onCancel={() => setLabelling(null)}
          onConfirm={async comment => {
            setBusy(true)
            setError(null)
            try {
              await createLabel(projectId, comment, labelling.toV)
              setLabelling(null)
              await load()
            } catch (thrown) {
              setError(messageFor(thrown))
            } finally {
              setBusy(false)
            }
          }}
          busy={busy}
        />
      ) : null}
    </div>
  )
}

function ChangeMark({ operation }: { operation?: FileChange['operation'] }) {
  const mark =
    operation === 'added'
      ? { glyph: '+', className: 'history-change-added' }
      : operation === 'removed'
        ? { glyph: '−', className: 'history-change-removed' }
        : operation === 'renamed'
          ? { glyph: '→', className: 'history-change-renamed' }
          : { glyph: '·', className: 'history-change-edited' }
  return (
    <span aria-hidden className={cx('history-change-mark', mark.className)}>
      {mark.glyph}
    </span>
  )
}

function DiffPiece({ chunk }: { chunk: DiffChunk }) {
  if (chunk.i !== undefined) {
    return <ins className="history-diff-insert">{chunk.i}</ins>
  }
  if (chunk.d !== undefined) {
    return <del className="history-diff-delete">{chunk.d}</del>
  }
  return <span>{chunk.u ?? ''}</span>
}

function LabelDialog({
  version,
  busy,
  onCancel,
  onConfirm,
}: {
  version: number
  busy: boolean
  onCancel: () => void
  onConfirm: (comment: string) => void
}) {
  const { t } = useTranslation()
  const [comment, setComment] = useState('')

  return (
    <OLModal show onHide={onCancel}>
      <form
        onSubmit={event => {
          event.preventDefault()
          if (comment.trim() && !busy) {
            onConfirm(comment.trim())
          }
        }}
      >
        <OLModalHeader>
          <OLModalTitle>{t('history_add_label')}</OLModalTitle>
        </OLModalHeader>
        <OLModalBody>
          <OLFormGroup controlId="history-label">
            <OLFormLabel>{t('history_new_label_name')}</OLFormLabel>
            <OLFormControl
              autoFocus
              type="text"
              value={comment}
              onChange={event => setComment(event.target.value)}
            />
            <OLFormText>{t('history_label_this_version')}</OLFormText>
          </OLFormGroup>
        </OLModalBody>
        <OLModalFooter>
          <Button variant="secondary" onClick={onCancel} disabled={busy}>
            {t('cancel')}
          </Button>
          <Button variant="primary" type="submit" isLoading={busy} disabled={!comment.trim()}>
            {t('save')}
          </Button>
        </OLModalFooter>
      </form>
    </OLModal>
  )
}

export default HistoryView
