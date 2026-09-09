'use client'

/**
 * The visual mode: LaTeX shown as what it produces.
 *
 * Not a different editor. The document is the same text and every edit is the
 * same operation -- what changes is that the markup of the commands somebody
 * uses constantly is hidden and their effect is drawn instead. `\textbf{loud}`
 * reads as **loud**, a section heading reads as a heading, an item reads as a
 * bullet.
 *
 * Two rules keep this honest:
 *
 *   - only commands whose effect can actually be shown are hidden. Anything
 *     else stays as source, because hiding markup whose meaning is not being
 *     conveyed is just losing information.
 *   - nothing is hidden where the cursor is. Editing text you cannot see is
 *     the failure mode of every rich-text view of a markup language, and
 *     revealing the line being worked on is what avoids it.
 */

import { EditorState, Range, StateField, type Extension } from '@codemirror/state'
import { Decoration, EditorView, WidgetType, type DecorationSet } from '@codemirror/view'

/** A command that wraps text and can be drawn as a style. */
const STYLES: Record<string, string> = {
  textbf: 'cm-visual-bold',
  textit: 'cm-visual-italic',
  emph: 'cm-visual-italic',
  texttt: 'cm-visual-mono',
  textsc: 'cm-visual-smallcaps',
  underline: 'cm-visual-underline',
}

/** A sectioning command and the size it is drawn at. */
const HEADINGS: Record<string, string> = {
  part: 'cm-visual-h1',
  chapter: 'cm-visual-h1',
  section: 'cm-visual-h2',
  subsection: 'cm-visual-h3',
  subsubsection: 'cm-visual-h4',
  paragraph: 'cm-visual-h5',
}

/** The bullet drawn in place of \item. */
class BulletWidget extends WidgetType {
  constructor(private readonly glyph: string) {
    super()
  }
  eq(other: BulletWidget) {
    return other.glyph === this.glyph
  }
  toDOM() {
    const span = document.createElement('span')
    span.className = 'cm-visual-bullet'
    span.textContent = this.glyph
    return span
  }
  ignoreEvent() {
    return true
  }
}

/** Finds the matching close brace for the `{` at `open`. */
function closingBrace(text: string, open: number): number {
  let depth = 0
  for (let i = open; i < text.length; i++) {
    const character = text[i]
    if (character === '\\') {
      i++
      continue
    }
    if (character === '{') {
      depth++
    } else if (character === '}') {
      depth--
      if (depth === 0) {
        return i
      }
    }
  }
  return -1
}

const COMMAND = /\\([a-zA-Z]+)\s*\{/g
const ITEM = /^(\s*)\\item\b[ \t]*/gm
const LIST_EDGE = /^[ \t]*\\(?:begin|end)\s*\{(itemize|enumerate|description)\}[ \t]*$/gm

/**
 * The lines the cursor is on, which are never hidden.
 *
 * A range rather than a line, because a selection can span several and all of
 * them are being worked on.
 */
function activeLines(state: EditorState): { from: number; to: number } {
  const selection = state.selection.main
  return {
    from: state.doc.lineAt(selection.from).from,
    to: state.doc.lineAt(selection.to).to,
  }
}

function decorationsFor(state: EditorState): DecorationSet {
  const text = state.doc.toString()
  const active = activeLines(state)
  const marks: Range<Decoration>[] = []

  const revealed = (from: number, to: number) =>
    !(to < active.from || from > active.to)

  // Sectioning and text styling: the command and its braces are hidden, and
  // what they contain is drawn as the command would draw it.
  COMMAND.lastIndex = 0
  let found: RegExpExecArray | null
  while ((found = COMMAND.exec(text))) {
    const name = found[1] ?? ''
    const style = STYLES[name]
    const heading = HEADINGS[name]
    if (!style && !heading) {
      continue
    }
    const open = found.index + found[0].length - 1
    const close = closingBrace(text, open)
    if (close === -1) {
      continue
    }
    if (revealed(found.index, close)) {
      continue
    }
    marks.push(Decoration.replace({}).range(found.index, open + 1))
    if (close > open + 1) {
      marks.push(
        Decoration.mark({ class: heading ?? style ?? '' }).range(open + 1, close)
      )
    }
    marks.push(Decoration.replace({}).range(close, close + 1))
  }

  // \item becomes a bullet.
  ITEM.lastIndex = 0
  while ((found = ITEM.exec(text))) {
    const from = found.index + (found[1] ?? '').length
    const to = found.index + found[0].length
    if (revealed(from, to)) {
      continue
    }
    marks.push(
      Decoration.replace({ widget: new BulletWidget('•') }).range(from, to)
    )
  }

  // The \begin and \end of a list add nothing a reader needs once the items
  // are bullets, so the whole line goes.
  LIST_EDGE.lastIndex = 0
  while ((found = LIST_EDGE.exec(text))) {
    const from = found.index
    const to = found.index + found[0].length
    if (revealed(from, to)) {
      continue
    }
    marks.push(Decoration.replace({}).range(from, to))
  }

  // A decoration set has to be built in position order, and replacements have
  // to come before marks that start at the same place.
  marks.sort((a, b) => a.from - b.from || a.to - b.to)
  return Decoration.set(marks, true)
}

const visualField = StateField.define<DecorationSet>({
  create(state) {
    return decorationsFor(state)
  },
  update(decorations, transaction) {
    // Rebuilt on a selection change as well as an edit, because what is
    // revealed depends on where the cursor is.
    if (transaction.docChanged || transaction.selection) {
      return decorationsFor(transaction.state)
    }
    return decorations
  },
  provide: field => EditorView.decorations.from(field),
})

const visualTheme = EditorView.baseTheme({
  '.cm-visual-bold': { fontWeight: '700' },
  '.cm-visual-italic': { fontStyle: 'italic' },
  '.cm-visual-mono': { fontFamily: 'var(--font-mono)' },
  '.cm-visual-smallcaps': { fontVariant: 'small-caps' },
  '.cm-visual-underline': { textDecoration: 'underline' },
  '.cm-visual-h1': { fontSize: '1.7em', fontWeight: '700', lineHeight: '1.3' },
  '.cm-visual-h2': { fontSize: '1.45em', fontWeight: '700', lineHeight: '1.3' },
  '.cm-visual-h3': { fontSize: '1.25em', fontWeight: '700', lineHeight: '1.3' },
  '.cm-visual-h4': { fontSize: '1.1em', fontWeight: '700' },
  '.cm-visual-h5': { fontWeight: '700' },
  '.cm-visual-bullet': {
    color: 'var(--content-secondary)',
    paddingRight: '0.5em',
  },
})

/** The extension, for the compartment that turns visual mode on and off. */
export function visualMode(): Extension {
  return [visualField, visualTheme]
}
