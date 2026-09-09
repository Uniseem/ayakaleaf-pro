import { ChangeSet, Text, Transaction, TransactionSpec } from '@codemirror/state'
import { EditorView, ViewPlugin } from '@codemirror/view'

/**
 * The bridge between the editor and the document session that keeps it in
 * step with everybody else.
 *
 * Typing goes out: every transaction that changed the document and was not
 * marked remote is reported, as the whole new text, to the session, which
 * works out the operation. Edits from elsewhere come in through
 * `applyRemoteText`, which turns the difference between what the editor
 * holds and what the session now holds into one change carrying the remote
 * annotation, so nothing here reports it back out again.
 */
export const realtime = (onLocalChange: (text: string) => void) => {
  return ViewPlugin.define(() => ({
    update(update) {
      if (!update.docChanged) {
        return
      }
      const local = update.transactions.some(tr => !tr.annotation(Transaction.remote) && tr.docChanged)
      if (local) {
        onLocalChange(update.state.doc.toString())
      }
    },
  }))
}

/**
 * The smallest single change that turns the editor's text into `next`.
 *
 * Only the middle differs once the shared prefix and suffix are taken off,
 * which is exact for any one edit and close enough for several at once.
 */
export const remoteTextChange = (held: Text, next: string): TransactionSpec | null => {
  const current = held.toString()
  if (current === next) {
    return null
  }
  let start = 0
  const shortest = Math.min(current.length, next.length)
  while (start < shortest && current[start] === next[start]) {
    start++
  }
  let end = 0
  while (end < shortest - start && current[current.length - 1 - end] === next[next.length - 1 - end]) {
    end++
  }
  const changes = ChangeSet.of({ from: start, to: current.length - end, insert: next.slice(start, next.length - end) }, current.length)
  return {
    changes,
    annotations: [Transaction.remote.of(true), Transaction.addToHistory.of(false)],
  }
}

export const applyRemoteText = (view: EditorView, next: string) => {
  const spec = remoteTextChange(view.state.doc, next)
  if (spec) {
    view.dispatch(spec)
  }
}
