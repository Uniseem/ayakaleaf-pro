'use client'

/**
 * What happened to this project.
 *
 * Versions down the left, the chosen one's changes on the right. The history
 * service has already grouped thousands of keystrokes into sittings of work,
 * which is what makes the list readable at all -- a list of operations would
 * be a list of individual characters.
 *
 * Restoring writes the old content as a new edit rather than rewinding.
 * Undoing a mistake is another thing that happened, and a history that hides
 * it is one you cannot undo an undo in.
 */

import {
  Button,
  Chip,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ScrollShadow,
  Spinner,
} from '@heroui/react'
import { useCallback, useEffect, useState } from 'react'
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
        setUpdates(previous =>
          before ? [...previous, ...answer.updates] : answer.updates
        )
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
    <div className="flex h-full min-h-0 flex-col bg-background">
      <header className="flex items-center gap-2 border-b border-divider px-3 py-2">
        <span className="text-sm font-medium">History</span>
        <div className="flex-1" />
        <Button size="sm" variant="light" className="h-7" onPress={layout.restoreView}>
          Back to the editor
        </Button>
      </header>

      {error ? (
        <p className="border-b border-divider bg-danger-50 px-3 py-2 text-xs text-danger">
          {error}
        </p>
      ) : null}

      <div className="flex min-h-0 flex-1">
        <aside className="flex w-72 shrink-0 flex-col border-r border-divider">
          <ScrollShadow className="min-h-0 flex-1">
            {loading ? (
              <div className="flex justify-center py-8">
                <Spinner size="sm" />
              </div>
            ) : updates.length === 0 ? (
              <p className="p-4 text-xs text-default-400">
                Nothing yet. A version appears here after the first time
                somebody stops typing.
              </p>
            ) : (
              <ul className="divide-y divide-divider">
                {updates.map(update => (
                  <li key={`${update.fromV}-${update.toV}`}>
                    <button
                      type="button"
                      onClick={() => setChosen(update)}
                      className={`w-full px-3 py-2 text-left ${
                        chosen?.toV === update.toV ? 'bg-default-100' : 'hover:bg-default-50'
                      }`}
                    >
                      <div className="flex items-center gap-1">
                        <time className="text-xs font-medium">
                          {update.meta.end_ts
                            ? new Date(update.meta.end_ts).toLocaleString()
                            : `version ${update.toV}`}
                        </time>
                        {update.labels?.length ? (
                          <Chip size="sm" variant="flat" className="h-4 text-[10px]">
                            {update.labels[0]?.comment}
                          </Chip>
                        ) : null}
                      </div>
                      <p className="truncate text-[11px] text-default-500">
                        {authorsOf(update)}
                      </p>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </ScrollShadow>
          {nextBefore ? (
            <Button
              size="sm"
              variant="light"
              className="m-2 h-7 text-xs"
              onPress={() => void load(nextBefore)}
            >
              Load older
            </Button>
          ) : null}
        </aside>

        <section className="flex min-w-0 flex-1 flex-col">
          {chosen ? (
            <>
              <div className="flex items-center gap-2 border-b border-divider px-3 py-2">
                <span className="text-xs text-default-500">
                  Version {chosen.fromV} to {chosen.toV}
                </span>
                <div className="flex-1" />
                {canWrite ? (
                  <>
                    <Button
                      size="sm"
                      variant="light"
                      className="h-7 text-xs"
                      onPress={() => setLabelling(chosen)}
                    >
                      Label this version
                    </Button>
                    {path ? (
                      <Button
                        size="sm"
                        variant="flat"
                        className="h-7 text-xs"
                        isLoading={busy}
                        onPress={async () => {
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
                        Restore this file
                      </Button>
                    ) : null}
                  </>
                ) : null}
              </div>

              <div className="flex min-h-0 flex-1">
                <nav className="w-56 shrink-0 border-r border-divider">
                  <ScrollShadow className="h-full">
                    {changes === null ? (
                      <div className="flex justify-center py-6">
                        <Spinner size="sm" />
                      </div>
                    ) : changes.length === 0 ? (
                      <p className="p-3 text-xs text-default-400">
                        No files changed.
                      </p>
                    ) : (
                      <ul className="p-1">
                        {changes.map(change => (
                          <li key={change.pathname}>
                            <button
                              type="button"
                              onClick={() => setPath(change.pathname)}
                              className={`flex w-full items-center gap-1.5 truncate rounded px-2 py-1 text-left text-xs ${
                                path === change.pathname
                                  ? 'bg-default-200'
                                  : 'hover:bg-default-100'
                              }`}
                              title={change.pathname}
                            >
                              <ChangeMark operation={change.operation} />
                              <span className="truncate">{change.pathname}</span>
                            </button>
                          </li>
                        ))}
                      </ul>
                    )}
                  </ScrollShadow>
                </nav>

                <ScrollShadow className="min-h-0 flex-1">
                  {diff === null ? (
                    <div className="flex justify-center py-8">
                      <Spinner size="sm" />
                    </div>
                  ) : diff.length === 0 ? (
                    <p className="p-4 text-xs text-default-400">
                      Nothing to show for this file.
                    </p>
                  ) : (
                    <pre className="whitespace-pre-wrap break-words p-3 font-mono text-[11px] leading-relaxed">
                      {diff.map((chunk, index) => (
                        <DiffPiece key={index} chunk={chunk} />
                      ))}
                    </pre>
                  )}
                </ScrollShadow>
              </div>
            </>
          ) : (
            <p className="p-6 text-sm text-default-400">
              Choose a version on the left.
            </p>
          )}
        </section>
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
      ? { glyph: '+', className: 'text-success' }
      : operation === 'removed'
        ? { glyph: '−', className: 'text-danger' }
        : operation === 'renamed'
          ? { glyph: '→', className: 'text-warning-600' }
          : { glyph: '·', className: 'text-default-400' }
  return (
    <span aria-hidden className={`w-3 shrink-0 text-center ${mark.className}`}>
      {mark.glyph}
    </span>
  )
}

function DiffPiece({ chunk }: { chunk: DiffChunk }) {
  if (chunk.i !== undefined) {
    return <ins className="bg-success-100 text-success-800 no-underline">{chunk.i}</ins>
  }
  if (chunk.d !== undefined) {
    return <del className="bg-danger-100 text-danger-800">{chunk.d}</del>
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
  const [comment, setComment] = useState('')
  return (
    <Modal isOpen onClose={onCancel} size="sm">
      <ModalContent>
        <form
          onSubmit={event => {
            event.preventDefault()
            if (comment.trim() && !busy) {
              onConfirm(comment.trim())
            }
          }}
        >
          <ModalHeader>Label version {version}</ModalHeader>
          <ModalBody>
            <Input
              autoFocus
              label="Name"
              placeholder="Submitted to the journal"
              value={comment}
              onValueChange={setComment}
            />
            <p className="text-xs text-default-500">
              A labelled version is easy to come back to later.
            </p>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onCancel} isDisabled={busy}>
              Cancel
            </Button>
            <Button
              color="primary"
              type="submit"
              isLoading={busy}
              isDisabled={!comment.trim()}
            >
              Save
            </Button>
          </ModalFooter>
        </form>
      </ModalContent>
    </Modal>
  )
}
