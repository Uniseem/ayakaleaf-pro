'use client'

/**
 * Whether edits are made, suggested, or not made at all.
 *
 * Three modes, which is the shape the original has and the shape the feature
 * needs: somebody may be the author, somebody may have been asked to comment
 * on a draft without rewriting it, and somebody may just be reading.
 *
 * "Suggesting" is not a different way of editing. The same operation is sent;
 * a seed goes with it, and the server records the edit as a tracked change
 * rather than applying it. That is the whole mechanism, and keeping it in one
 * field is what stops it from being half-on somewhere.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import {
  acceptChanges as acceptOnServer,
  getRanges,
  newTrackingSeed,
  type Ranges,
} from '@/lib/ranges'
import { messageFor } from '@/lib/api'
import { usePersistedState } from '@/lib/hooks'
import { useProject } from './project-context'
import { useEditor } from './editor-context'

export type ReviewMode = 'editing' | 'suggesting' | 'viewing'

export type ReviewValue = {
  mode: ReviewMode
  setMode: (mode: ReviewMode) => void
  /** The seed to stamp suggestions with, or null when not suggesting. */
  trackingSeed: string | null

  ranges: Ranges
  loading: boolean
  error: string | null
  /** Reads the markers back from the server. */
  refresh: () => Promise<void>

  accept: (changeIds: string[]) => Promise<void>
  reject: (changeIds: string[]) => Promise<void>
}

const ReviewContext = createContext<ReviewValue | undefined>(undefined)

const EMPTY: Ranges = { changes: [], comments: [] }

export function ReviewProvider({ children }: { children: ReactNode }) {
  const { projectId, canWrite } = useProject()
  const editor = useEditor()

  const [stored, setStored] = usePersistedState<ReviewMode>(
    `ide.reviewMode.${projectId}`,
    'editing'
  )
  const [ranges, setRanges] = useState<Ranges>(EMPTY)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // One seed for the session, so a run of typing is one tracked change rather
  // than one per keystroke.
  const seed = useRef<string>(newTrackingSeed())

  // Somebody who cannot write cannot edit or suggest, whatever was remembered.
  const mode: ReviewMode = canWrite ? stored : 'viewing'
  const docId = editor.current?.id ?? null
  const trackingSeed = mode === 'suggesting' ? seed.current : null

  // Told to the open document, so the next edit is stamped or not.
  const { setTracking } = editor
  useEffect(() => {
    setTracking(trackingSeed)
  }, [setTracking, trackingSeed])

  const refresh = useCallback(async () => {
    if (!docId) {
      setRanges(EMPTY)
      return
    }
    setLoading(true)
    setError(null)
    try {
      setRanges(await getRanges(projectId, docId))
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setLoading(false)
    }
  }, [projectId, docId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  // The markers move with every edit, and the server is the one that moves
  // them. Re-reading after a pause is chattier than transforming them here,
  // and it is the only version that cannot drift: a marker a few characters
  // out highlights the wrong words and looks exactly like one that is right.
  useEffect(() => {
    if (!docId) {
      return
    }
    const timer = setTimeout(() => void refresh(), 1500)
    return () => clearTimeout(timer)
  }, [docId, editor.current?.content, refresh])

  const accept = useCallback(
    async (changeIds: string[]) => {
      if (!docId || changeIds.length === 0) {
        return
      }
      await acceptOnServer(projectId, docId, changeIds)
      // Accepting happens over HTTP, under document-updater's own lock, so
      // the editing session knows nothing about it and would keep sending
      // operations against the version it had before -- every one of which
      // the server refuses. Re-reading is what puts the two back in step.
      await editor.reload()
      await refresh()
    },
    [projectId, docId, editor, refresh]
  )

  /**
   * Undoes a suggestion.
   *
   * There is no endpoint for this, and there should not be: rejecting an
   * insertion means deleting text and rejecting a deletion means typing it
   * back, and both are ordinary edits. Sent as edits, they go through the same
   * transformation as everything else and cannot disagree with what anybody
   * else is doing at the time.
   */
  const reject = useCallback(
    async (changeIds: string[]) => {
      if (!docId) {
        return
      }
      const wanted = ranges.changes.filter(change => changeIds.includes(change.id))
      if (wanted.length === 0) {
        return
      }
      window.dispatchEvent(
        new CustomEvent('ide:reject-changes', { detail: { changes: wanted } })
      )
      // The edit has to land before the markers are worth reading again.
      setTimeout(() => void refresh(), 600)
    },
    [docId, ranges.changes, refresh]
  )

  const value = useMemo<ReviewValue>(
    () => ({
      mode,
      setMode: setStored,
      trackingSeed,
      ranges,
      loading,
      error,
      refresh,
      accept,
      reject,
    }),
    [mode, setStored, trackingSeed, ranges, loading, error, refresh, accept, reject]
  )

  return <ReviewContext.Provider value={value}>{children}</ReviewContext.Provider>
}

export function useReview(): ReviewValue {
  const value = useContext(ReviewContext)
  if (!value) {
    throw new Error('useReview must be used inside a ReviewProvider')
  }
  return value
}
