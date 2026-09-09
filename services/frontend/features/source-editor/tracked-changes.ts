'use client'

/**
 * Showing tracked changes in the text.
 *
 * An insertion is text that is in the document, so it is a mark over the range
 * it occupies. A deletion is text that is *not* in the document any more, so
 * there is nothing to mark: it is drawn as a widget at the position it was
 * removed from, showing what it said.
 *
 * That asymmetry is the whole difficulty of rendering this, and it comes from
 * the storage model rather than from a choice made here.
 */

import { StateEffect, StateField, type Extension } from '@codemirror/state'
import { Decoration, EditorView, WidgetType, type DecorationSet } from '@codemirror/view'
import { changeLength, isDeletion, isInsertion, type TrackedChange } from '@/lib/ranges'

/** Replaces the changes the editor is showing. */
export const setTrackedChanges = StateEffect.define<TrackedChange[]>()

/** The text of a deletion, drawn where it used to be. */
class DeletionWidget extends WidgetType {
  constructor(private readonly text: string) {
    super()
  }

  eq(other: DeletionWidget) {
    return other.text === this.text
  }

  toDOM() {
    const span = document.createElement('span')
    span.className = 'cm-tracked-deletion'
    // Newlines in deleted text would otherwise close the line up and make the
    // widget look like it belongs to the wrong paragraph.
    span.textContent = this.text.replace(/\n/g, '¶')
    span.title = 'Deleted'
    return span
  }

  ignoreEvent() {
    return true
  }
}

const insertionMark = Decoration.mark({ class: 'cm-tracked-insertion' })

function decorationsFor(changes: TrackedChange[], length: number): DecorationSet {
  const marks = []
  // Sorted, because a decoration set has to be built in position order.
  const ordered = [...changes].sort((a, b) => a.op.p - b.op.p)

  for (const change of ordered) {
    const at = change.op.p
    if (at < 0 || at > length) {
      // A marker whose position is past the end of the document is one that
      // has drifted. Drawing it somewhere arbitrary would be worse than not
      // drawing it.
      continue
    }
    if (isInsertion(change)) {
      const to = Math.min(at + changeLength(change), length)
      if (to > at) {
        marks.push(insertionMark.range(at, to))
      }
    } else if (isDeletion(change)) {
      marks.push(
        Decoration.widget({
          widget: new DeletionWidget(change.op.d),
          side: 1,
        }).range(at)
      )
    }
  }
  return Decoration.set(marks, true)
}

const trackedChangesField = StateField.define<DecorationSet>({
  create() {
    return Decoration.none
  },
  update(decorations, transaction) {
    // Moved with the text first, so that a decoration stays on its words
    // between one set of markers arriving and the next.
    let updated = decorations.map(transaction.changes)
    for (const effect of transaction.effects) {
      if (effect.is(setTrackedChanges)) {
        updated = decorationsFor(effect.value, transaction.state.doc.length)
      }
    }
    return updated
  },
  provide: field => EditorView.decorations.from(field),
})

const trackedChangesTheme = EditorView.baseTheme({
  '.cm-tracked-insertion': {
    backgroundColor: 'var(--tracked-insert-background)',
    borderBottom: '1px solid var(--tracked-insert-border)',
  },
  '.cm-tracked-deletion': {
    backgroundColor: 'var(--tracked-delete-background)',
    color: 'var(--tracked-delete-foreground)',
    textDecoration: 'line-through',
    opacity: 0.85,
    padding: '0 1px',
  },
})

export function trackedChanges(): Extension {
  return [trackedChangesField, trackedChangesTheme]
}

/**
 * The edit that undoes a tracked change.
 *
 * Rejecting an insertion deletes the text it added; rejecting a deletion types
 * the text back. Both are ordinary edits, which is what makes them safe: they
 * go through the same transformation as everything else.
 *
 * Applied from the end backwards, so that each change's position still refers
 * to the document the one before it left behind.
 */
export function rejectionChanges(
  changes: TrackedChange[],
  length: number
): Array<{ from: number; to: number; insert: string }> {
  return [...changes]
    .sort((a, b) => b.op.p - a.op.p)
    .flatMap(change => {
      const at = change.op.p
      if (at < 0 || at > length) {
        return []
      }
      if (isInsertion(change)) {
        return [{ from: at, to: Math.min(at + change.op.i.length, length), insert: '' }]
      }
      if (isDeletion(change)) {
        return [{ from: at, to: at, insert: change.op.d }]
      }
      return []
    })
}
