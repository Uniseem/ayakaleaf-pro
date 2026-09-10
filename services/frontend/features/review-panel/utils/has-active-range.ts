import type { Ranges } from '@/lib/ranges'
import type { Threads } from '../contexts/threads-context'

/**
 * Whether there is anything in this document worth showing the panel for.
 *
 * Undefined rather than false while the data is still loading, so the caller
 * can tell "nothing here" from "not known yet" and not flash the panel open
 * and shut on the way in.
 */
export const hasActiveRange = (
  ranges: Ranges | undefined,
  threads: Threads | undefined
): boolean | undefined => {
  if (!ranges || !threads) {
    return undefined
  }

  if (ranges.changes.length > 0) {
    return true
  }

  for (const comment of ranges.comments) {
    const threadId = comment.op.t ?? comment.op.tid
    const thread = threadId ? threads[threadId] : undefined
    if (thread && !thread.resolved) {
      return true
    }
  }

  return false
}
