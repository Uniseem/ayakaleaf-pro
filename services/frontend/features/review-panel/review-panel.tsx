'use client'

/**
 * Comments on the document.
 *
 * A thread is a conversation about a place in the text. The place itself lives
 * in the document's own data, so it moves when the text around it is edited
 * and nothing here has to be rewritten -- which is what makes a comment
 * survive somebody typing a paragraph above it.
 *
 * Resolved threads are hidden rather than deleted. "Done" and "never happened"
 * are different things, and a review where the second is the only option is
 * one nobody trusts.
 */

import {
  Avatar,
  Button,
  Chip,
  ScrollShadow,
  Spinner,
  Textarea,
} from '@heroui/react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  comment as postComment,
  deleteThread,
  listThreads,
  reopenThread,
  resolveThread,
  type Thread,
} from '@/lib/comments'
import { nameOf } from '@/lib/chat'
import { messageFor } from '@/lib/api'
import { useProject } from '@/features/ide/contexts/project-context'
import { useReview } from '@/features/ide/contexts/review-context'
import { changeLength, isInsertion } from '@/lib/ranges'

export function ReviewPanel() {
  const { projectId, canWrite, canReview } = useProject()

  const [threads, setThreads] = useState<Record<string, Thread>>({})
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showResolved, setShowResolved] = useState(false)
  const [replyingTo, setReplyingTo] = useState<string | null>(null)
  const [draft, setDraft] = useState('')

  const load = useCallback(async () => {
    setError(null)
    try {
      setThreads(await listThreads(projectId))
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    void load()
  }, [load])

  const run = useCallback(
    async (work: () => Promise<unknown>) => {
      setBusy(true)
      setError(null)
      try {
        await work()
        await load()
      } catch (thrown) {
        setError(messageFor(thrown))
      } finally {
        setBusy(false)
      }
    },
    [load]
  )

  const { open, resolved } = useMemo(() => {
    const all = Object.values(threads)
    return {
      open: all.filter(each => !each.resolved),
      resolved: all.filter(each => each.resolved),
    }
  }, [threads])

  const shown = showResolved ? resolved : open

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex items-center gap-2 border-b border-divider px-3 py-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-default-500">
          Review
        </span>
        <div className="flex-1" />
        <button
          type="button"
          className="text-[11px] text-default-500 underline underline-offset-2 hover:text-foreground"
          onClick={() => setShowResolved(value => !value)}
        >
          {showResolved ? `Open (${open.length})` : `Resolved (${resolved.length})`}
        </button>
      </header>

      {error ? (
        <p className="border-b border-divider bg-danger-50 px-3 py-2 text-xs text-danger">
          {error}
        </p>
      ) : null}

      <TrackedChanges />

      <ScrollShadow className="min-h-0 flex-1">
        {loading ? (
          <div className="flex justify-center py-8">
            <Spinner size="sm" />
          </div>
        ) : shown.length === 0 ? (
          <p className="p-4 text-xs text-default-400">
            {showResolved
              ? 'Nothing resolved yet.'
              : canReview
                ? 'No comments. Select some text in the editor and add one to start a thread here.'
                : 'No comments.'}
          </p>
        ) : (
          <ul className="divide-y divide-divider">
            {shown.map(thread => (
              <li key={thread.id} className="p-3">
                <ul className="flex flex-col gap-2">
                  {thread.messages.map(message => (
                    <li key={message.id} className="flex gap-2">
                      <Avatar
                        name={nameOf(message.user)}
                        size="sm"
                        className="h-5 w-5 shrink-0 text-[9px]"
                      />
                      <div className="min-w-0 flex-1">
                        <div className="flex items-baseline gap-2">
                          <span className="truncate text-[11px] font-medium">
                            {nameOf(message.user)}
                          </span>
                          <time className="text-[10px] text-default-400">
                            {new Date(message.timestamp).toLocaleString()}
                          </time>
                        </div>
                        <p className="whitespace-pre-wrap break-words text-xs text-default-700">
                          {message.content}
                        </p>
                      </div>
                    </li>
                  ))}
                </ul>

                {thread.resolved ? (
                  <div className="mt-2 flex items-center gap-2">
                    <Chip size="sm" variant="flat" color="success" className="h-5 text-[10px]">
                      Resolved
                      {thread.resolvedBy ? ` by ${nameOf(thread.resolvedBy)}` : ''}
                    </Chip>
                    {canReview ? (
                      <Button
                        size="sm"
                        variant="light"
                        className="h-6 min-w-0 px-2 text-[11px]"
                        isDisabled={busy}
                        onPress={() => void run(() => reopenThread(projectId, thread.id))}
                      >
                        Reopen
                      </Button>
                    ) : null}
                  </div>
                ) : canReview ? (
                  replyingTo === thread.id ? (
                    <form
                      className="mt-2"
                      onSubmit={event => {
                        event.preventDefault()
                        const content = draft.trim()
                        if (!content || busy) {
                          return
                        }
                        void run(async () => {
                          await postComment(projectId, thread.id, content)
                          setDraft('')
                          setReplyingTo(null)
                        })
                      }}
                    >
                      <Textarea
                        autoFocus
                        size="sm"
                        minRows={1}
                        maxRows={4}
                        value={draft}
                        onValueChange={setDraft}
                        placeholder="Reply"
                      />
                      <div className="mt-1 flex gap-1">
                        <Button
                          size="sm"
                          color="primary"
                          type="submit"
                          className="h-6 min-w-0 px-2 text-[11px]"
                          isDisabled={!draft.trim() || busy}
                        >
                          Reply
                        </Button>
                        <Button
                          size="sm"
                          variant="light"
                          className="h-6 min-w-0 px-2 text-[11px]"
                          onPress={() => {
                            setReplyingTo(null)
                            setDraft('')
                          }}
                        >
                          Cancel
                        </Button>
                      </div>
                    </form>
                  ) : (
                    <div className="mt-2 flex gap-1">
                      <Button
                        size="sm"
                        variant="light"
                        className="h-6 min-w-0 px-2 text-[11px]"
                        onPress={() => {
                          setReplyingTo(thread.id)
                          setDraft('')
                        }}
                      >
                        Reply
                      </Button>
                      <Button
                        size="sm"
                        variant="light"
                        className="h-6 min-w-0 px-2 text-[11px]"
                        isDisabled={busy}
                        onPress={() => void run(() => resolveThread(projectId, thread.id))}
                      >
                        Resolve
                      </Button>
                      {canWrite ? (
                        <Button
                          size="sm"
                          variant="light"
                          color="danger"
                          className="h-6 min-w-0 px-2 text-[11px]"
                          isDisabled={busy}
                          onPress={() => void run(() => deleteThread(projectId, thread.id))}
                        >
                          Delete
                        </Button>
                      ) : null}
                    </div>
                  )
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </ScrollShadow>
    </div>
  )
}


/**
 * The suggestions waiting on this document.
 *
 * Above the comment threads rather than mixed in with them: a suggestion is
 * something to decide about and a comment is something to answer, and a list
 * that mixes the two makes both easy to lose.
 */
function TrackedChanges() {
  const review = useReview()
  const { canWrite } = useProject()
  const [busy, setBusy] = useState(false)

  const changes = review.ranges.changes
  if (changes.length === 0) {
    return null
  }

  const run = async (work: () => Promise<unknown>) => {
    setBusy(true)
    try {
      await work()
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="border-b border-[var(--border-divider)]">
      <header className="flex items-center gap-2 px-3 py-2">
        <span className="text-[12px] font-semibold uppercase leading-4 tracking-wide text-[var(--content-secondary)]">
          Suggestions ({changes.length})
        </span>
        {canWrite ? (
          <div className="ml-auto flex gap-1">
            <button
              type="button"
              disabled={busy}
              onClick={() => void run(() => review.accept(changes.map(c => c.id)))}
              className="rounded-[4px] px-1.5 py-0.5 text-[11px] leading-4 text-[var(--content-positive)] hover:bg-[var(--hover-interaction)] disabled:opacity-50"
            >
              Accept all
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={() => void run(() => review.reject(changes.map(c => c.id)))}
              className="rounded-[4px] px-1.5 py-0.5 text-[11px] leading-4 text-[var(--content-danger)] hover:bg-[var(--hover-interaction)] disabled:opacity-50"
            >
              Reject all
            </button>
          </div>
        ) : null}
      </header>

      <ul className="max-h-56 overflow-auto">
        {changes.map(change => {
          const added = isInsertion(change)
          const text = added ? change.op.i : (change.op as { d: string }).d
          return (
            <li
              key={change.id}
              className="flex items-start gap-2 px-3 py-2 text-[12px] leading-4"
            >
              <span
                className={`mt-px shrink-0 font-medium ${
                  added ? 'text-[var(--content-positive)]' : 'text-[var(--content-danger)]'
                }`}
              >
                {added ? 'Added' : 'Removed'}
              </span>
              <span
                className={`min-w-0 flex-1 truncate font-mono ${
                  added ? '' : 'line-through'
                }`}
                title={text}
              >
                {oneLine(text).slice(0, 120)}
              </span>
              <span className="shrink-0 text-[var(--content-secondary)]">
                {changeLength(change)}
              </span>
              {canWrite ? (
                <span className="flex shrink-0 gap-0.5">
                  <button
                    type="button"
                    aria-label="Accept"
                    title="Accept"
                    disabled={busy}
                    onClick={() => void run(() => review.accept([change.id]))}
                    className="flex h-5 w-5 items-center justify-center rounded-[4px] text-[var(--content-positive)] hover:bg-[var(--hover-interaction)] disabled:opacity-50"
                  >
                    ✓
                  </button>
                  <button
                    type="button"
                    aria-label="Reject"
                    title="Reject"
                    disabled={busy}
                    onClick={() => void run(() => review.reject([change.id]))}
                    className="flex h-5 w-5 items-center justify-center rounded-[4px] text-[var(--content-danger)] hover:bg-[var(--hover-interaction)] disabled:opacity-50"
                  >
                    ✕
                  </button>
                </span>
              ) : null}
            </li>
          )
        })}
      </ul>
    </section>
  )
}

/**
 * A change's text on one line.
 *
 * A suggestion can span paragraphs, and a list row that grows to fit one
 * pushes every other row off the screen. The pilcrow stands in for the break.
 */
function oneLine(text: string): string {
  return text.split('\n').join('¶')
}
