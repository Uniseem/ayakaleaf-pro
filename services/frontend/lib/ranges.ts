/**
 * Tracked changes and comment anchors on a document.
 *
 * A tracked change is an edit recorded rather than applied: the text carries a
 * marker saying who wrote it and when, and somebody decides later whether it
 * stays. An insertion's text is in the document and marked; a deletion's text
 * is *not* in the document -- it was removed, and the marker holds a copy so
 * it can be shown and put back.
 *
 * The positions are only meaningful against one version of the document, and
 * every edit moves them, which is why they are read back from the server
 * rather than kept in step here: a marker that has drifted highlights the
 * wrong words, and looks exactly like one that has not.
 */

import { api } from './api'

/** An insertion: `i` is the text, which is present in the document. */
export type InsertOp = { p: number; i: string }
/** A deletion: `d` is the text, which is no longer in the document. */
export type DeleteOp = { p: number; d: string }
/** A comment anchor: `c` is the text it covers, `t` the thread. */
export type CommentOp = { p: number; c: string; t?: string; tid?: string }

export type TrackedChange = {
  id: string
  op: InsertOp | DeleteOp
  metadata?: { user_id?: string; ts?: string }
}

export type CommentAnchor = {
  id: string
  op: CommentOp
  metadata?: { user_id?: string; ts?: string }
}

export type Ranges = {
  changes: TrackedChange[]
  comments: CommentAnchor[]
}

export function isInsertion(
  change: TrackedChange
): change is TrackedChange & { op: InsertOp } {
  return 'i' in change.op
}

/**
 * The counterpart guard.
 *
 * Needed because narrowing one branch of a union does not narrow the other:
 * TypeScript knows what `isInsertion` proves, and nothing about what its being
 * false proves, so the deletion case has to say so itself.
 */
export function isDeletion(
  change: TrackedChange
): change is TrackedChange & { op: DeleteOp } {
  return 'd' in change.op
}

/** How long the change's text is, in either direction. */
export function changeLength(change: TrackedChange): number {
  return isInsertion(change) ? change.op.i.length : (change.op as DeleteOp).d.length
}

export async function getRanges(
  projectId: string,
  docId: string
): Promise<Ranges> {
  const answer = await api<{ ranges: Partial<Ranges> | null }>(
    `/api/projects/${projectId}/documents/${docId}/changes`
  )
  return {
    changes: answer.ranges?.changes ?? [],
    comments: answer.ranges?.comments ?? [],
  }
}

/**
 * Accepts changes, so the text stands and the markers go.
 *
 * Done by the server under the same lock edits take: an accept racing an edit
 * would otherwise lose one of the two.
 */
export function acceptChanges(
  projectId: string,
  docId: string,
  changeIds: string[]
): Promise<void> {
  return api<void>(
    `/api/projects/${projectId}/documents/${docId}/changes/accept`,
    { method: 'POST', body: { changeIds } }
  )
}

/**
 * A seed for the ids the server gives to tracked changes.
 *
 * The client supplies it so that a change made here can be recognised when it
 * comes back, and it is per-session rather than per-edit so a run of typing is
 * one change rather than one per character.
 */
export function newTrackingSeed(): string {
  const bytes = new Uint8Array(9)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')
}
