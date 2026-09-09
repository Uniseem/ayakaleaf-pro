'use client'

/**
 * The text editor.
 *
 * CodeMirror is driven directly rather than through a React wrapper. A wrapper
 * turns every keystroke into a React render and then reconciles a controlled
 * value back into the editor, which fights the editor's own undo history and
 * loses the cursor on any update that did not come from typing. So the view is
 * created once, kept in a ref, and told about changes; React renders the box
 * it lives in and nothing else.
 *
 * Switching documents replaces the state rather than the view, which keeps the
 * DOM and the scroll container and makes the switch instant.
 */

import { useCallback, useEffect, useMemo, useRef } from 'react'
import { EditorState, Compartment, type Extension } from '@codemirror/state'
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  drawSelection,
  dropCursor,
  rectangularSelection,
  crosshairCursor,
  placeholder as cmPlaceholder,
} from '@codemirror/view'
import {
  defaultKeymap,
  history,
  historyKeymap,
  indentWithTab,
  redo,
  undo,
} from '@codemirror/commands'
import {
  bracketMatching,
  foldGutter,
  foldKeymap,
  indentOnInput,
  indentUnit,
  StreamLanguage,
  syntaxHighlighting,
  defaultHighlightStyle,
  HighlightStyle,
  foldService,
} from '@codemirror/language'
import { stex } from '@codemirror/legacy-modes/mode/stex'
import { tags } from '@lezer/highlight'
import {
  autocompletion,
  closeBrackets,
  closeBracketsKeymap,
  completionKeymap,
} from '@codemirror/autocomplete'
import {
  highlightSelectionMatches,
  openSearchPanel,
  search,
  searchKeymap,
} from '@codemirror/search'
import { lintGutter, linter, lintKeymap } from '@codemirror/lint'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useConnection } from '@/features/ide/contexts/connection-context'
import { useSettings } from '@/features/ide/contexts/settings-context'
import { vim } from '@replit/codemirror-vim'
import { latexCompletions } from './completion'
import { latexDiagnostics } from './lint'
import { analyse } from './analyse'
import { rejectionChanges, setTrackedChanges, trackedChanges } from './tracked-changes'
import { visualMode } from './visual'
import { useReview } from '@/features/ide/contexts/review-context'

/**
 * The smallest single change that turns one string into another.
 *
 * Only the middle differs once the shared prefix and suffix are taken off,
 * which is true of any one edit and close enough for several at once.
 */
function smallestChange(
  from: string,
  to: string
): { from: number; to: number; insert: string } | null {
  if (from === to) {
    return null
  }
  let start = 0
  const shortest = Math.min(from.length, to.length)
  while (start < shortest && from[start] === to[start]) {
    start++
  }
  let end = 0
  while (
    end < shortest - start &&
    from[from.length - 1 - end] === to[to.length - 1 - end]
  ) {
    end++
  }
  return {
    from: start,
    to: from.length - end,
    insert: to.slice(start, to.length - end),
  }
}

/** Folding by environment and by section, which is what a LaTeX outline is. */
const latexFolding = foldService.of((state, lineStart, lineEnd) => {
  const source = state.doc.toString()
  const document = analyse(source)

  for (const span of document.environments) {
    if (span.from >= lineStart && span.from <= lineEnd && span.to !== null) {
      // Fold from the end of the \begin line to just before the \end.
      const openLine = state.doc.lineAt(span.from)
      const closeLine = state.doc.lineAt(span.to)
      if (closeLine.number > openLine.number) {
        return { from: openLine.to, to: closeLine.from - 1 }
      }
    }
  }

  for (let i = 0; i < document.sections.length; i++) {
    const section = document.sections[i]
    if (!section || section.from < lineStart || section.from > lineEnd) {
      continue
    }
    // A section runs until the next heading at the same level or shallower.
    const next = document.sections
      .slice(i + 1)
      .find(other => other.level <= section.level)
    const line = state.doc.lineAt(section.from)
    const end = next ? state.doc.line(next.line + 1).from - 1 : state.doc.length
    if (end > line.to) {
      return { from: line.to, to: end }
    }
  }
  return null
})

/** Colours for the LaTeX tokens stex produces. */
const latexHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--cm-command)' },
  { tag: tags.controlKeyword, color: 'var(--cm-command)' },
  { tag: tags.tagName, color: 'var(--cm-command)' },
  { tag: tags.comment, color: 'var(--cm-comment)', fontStyle: 'italic' },
  { tag: tags.string, color: 'var(--cm-string)' },
  { tag: tags.bracket, color: 'var(--cm-bracket)' },
  { tag: tags.brace, color: 'var(--cm-bracket)' },
  { tag: tags.atom, color: 'var(--cm-atom)' },
  { tag: tags.number, color: 'var(--cm-number)' },
  { tag: tags.variableName, color: 'var(--cm-variable)' },
  { tag: tags.typeName, color: 'var(--cm-type)' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.strong, fontWeight: 'bold' },
])

/**
 * The editor's own colours, as variables so the theme can be swapped without
 * rebuilding the extension.
 */
const baseTheme = EditorView.theme({
  '&': {
    height: '100%',
    backgroundColor: 'var(--cm-background)',
    color: 'var(--cm-foreground)',
  },
  '.cm-scroller': {
    fontFamily: 'inherit',
    lineHeight: 'inherit',
    overflow: 'auto',
  },
  '.cm-content': { caretColor: 'var(--cm-caret)', paddingBottom: '40vh' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--cm-caret)' },
  '.cm-gutters': {
    backgroundColor: 'var(--cm-gutter-background)',
    color: 'var(--cm-gutter-foreground)',
    border: 'none',
  },
  '.cm-activeLine': { backgroundColor: 'var(--cm-active-line)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--cm-active-line)' },
  '&.cm-focused .cm-selectionBackground, ::selection': {
    backgroundColor: 'var(--cm-selection)',
  },
  '.cm-selectionMatch': { backgroundColor: 'var(--cm-selection-match)' },
  '.cm-tooltip': {
    backgroundColor: 'var(--cm-tooltip-background)',
    border: '1px solid var(--cm-tooltip-border)',
    borderRadius: '6px',
  },
  '.cm-tooltip-autocomplete ul li[aria-selected]': {
    backgroundColor: 'var(--cm-selected)',
    color: 'var(--cm-foreground)',
  },
  '.cm-panels': {
    backgroundColor: 'var(--cm-gutter-background)',
    color: 'var(--cm-foreground)',
  },
})

export function SourceEditor() {
  const { current, change, rememberPosition, loading, editable, revision } = useEditor()
  const { markEdited, startCompile } = useCompile()
  const settings = useSettings()
  const { reportPosition } = useConnection()
  const review = useReview()

  const host = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  // Which document and which revision the view holds, to tell a switch and
  // an outside replacement from something that was just typed here.
  const showing = useRef<string | null>(null)
  const shown = useRef<number>(-1)

  // Compartments let one part of the configuration change without rebuilding
  // the rest -- toggling line wrapping must not discard the undo history.
  const compartments = useMemo(
    () => ({
      editable: new Compartment(),
      wrapping: new Compartment(),
      completion: new Compartment(),
      linting: new Compartment(),
      keys: new Compartment(),
      visual: new Compartment(),
    }),
    []
  )

  const onChange = useCallback(
    (text: string) => {
      change(text)
      markEdited()
    },
    [change, markEdited]
  )

  // Viewing is read-only whatever the person's access is: it is a mode they
  // chose, and an editor that ignores it is one that loses their place.
  const writable = editable && review.mode !== 'viewing'

  // Kept in a ref so the extension closes over something stable: rebuilding
  // the view when the callback changes would lose the cursor on every render.
  // Told where the cursor is, so other people see it. Throttled: a cursor
  // moving through a paragraph is one useful message, not forty.
  const lastReport = useRef(0)
  const report = useCallback(
    (head: number, state: EditorState) => {
      const now = Date.now()
      if (!current || now - lastReport.current < 300) {
        return
      }
      lastReport.current = now
      const line = state.doc.lineAt(head)
      reportPosition(current.id, line.number - 1, head - line.from)
    },
    [current, reportPosition]
  )

  const handlers = useRef({ onChange, rememberPosition, startCompile, report })
  handlers.current = { onChange, rememberPosition, startCompile, report }

  const extensions = useMemo<Extension[]>(
    () => [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      foldGutter(),
      latexFolding,
      drawSelection(),
      dropCursor(),
      EditorState.allowMultipleSelections.of(true),
      indentOnInput(),
      indentUnit.of('  '),
      bracketMatching(),
      closeBrackets(),
      rectangularSelection(),
      crosshairCursor(),
      highlightActiveLine(),
      highlightSelectionMatches(),
      search({ top: true }),
      lintGutter(),
      trackedChanges(),
      StreamLanguage.define(stex),
      syntaxHighlighting(latexHighlight),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      baseTheme,
      cmPlaceholder('Start typing, or open a file from the tree.'),
      keymap.of([
        {
          key: 'Mod-s',
          preventDefault: true,
          run: () => {
            handlers.current.startCompile()
            return true
          },
        },
        {
          key: 'Mod-Enter',
          preventDefault: true,
          run: () => {
            handlers.current.startCompile()
            return true
          },
        },
        {
          // The original's shortcut for the same jump.
          key: 'Mod-.',
          preventDefault: true,
          run: () => {
            window.dispatchEvent(new CustomEvent('ide:sync-to-pdf'))
            return true
          },
        },
        ...closeBracketsKeymap,
        ...defaultKeymap,
        ...searchKeymap,
        ...historyKeymap,
        ...foldKeymap,
        ...completionKeymap,
        ...lintKeymap,
        indentWithTab,
      ]),
      EditorView.updateListener.of(update => {
        if (update.docChanged) {
          handlers.current.onChange(update.state.doc.toString())
        }
        if (update.selectionSet || update.geometryChanged) {
          const head = update.state.selection.main.head
          handlers.current.rememberPosition(head, update.view.scrollDOM.scrollTop)
          handlers.current.report(head, update.state)
        }
      }),
    ],
    []
  )

  // Build the view once.
  useEffect(() => {
    if (!host.current || viewRef.current) {
      return
    }
    const created = new EditorView({
      state: EditorState.create({
        doc: '',
        extensions: [
          ...extensions,
          compartments.editable.of(EditorView.editable.of(false)),
          compartments.wrapping.of(EditorView.lineWrapping),
          compartments.completion.of([]),
          compartments.linting.of([]),
        ],
      }),
      parent: host.current,
    })
    viewRef.current = created
    return () => {
      created.destroy()
      viewRef.current = null
      showing.current = null
    }
  }, [extensions, compartments])

  // Load a document, or apply an edit that came from elsewhere.
  useEffect(() => {
    const editor = viewRef.current
    if (!editor || !current) {
      return
    }
    if (showing.current !== current.id || shown.current !== revision) {
      // A different document: replace the state, so undo history does not
      // cross from one file into another.
      editor.setState(
        EditorState.create({
          doc: current.content,
          extensions: [
            ...extensions,
            compartments.editable.of(EditorView.editable.of(writable)),
            compartments.wrapping.of(EditorView.lineWrapping),
            compartments.completion.of(
              settings.autoComplete
                ? autocompletion({ override: [latexCompletions] })
                : []
            ),
            compartments.linting.of(
              settings.syntaxValidation ? linter(latexDiagnostics) : []
            ),
            // Vim must come first in the list to take precedence over the
            // default keymap, which is why it is its own compartment rather
            // than part of the keymap above.
            compartments.keys.of(settings.keybindings === 'vim' ? vim() : []),
            compartments.visual.of(settings.mode === 'visual' ? visualMode() : []),
          ],
        })
      )
      showing.current = current.id
      shown.current = revision
      const at = Math.min(current.cursor, current.content.length)
      editor.dispatch({
        selection: { anchor: at },
        effects: EditorView.scrollIntoView(at, { y: 'center' }),
      })
      editor.focus()
      return
    }
    // Same document, and the text differs: somebody else edited it. Applied as
    // the smallest change that accounts for the difference rather than as a
    // whole-document replacement, because replacing the document moves the
    // cursor to the start and throws away the undo history -- on every
    // keystroke the other person types.
    const held = editor.state.doc.toString()
    if (held !== current.content) {
      const change = smallestChange(held, current.content)
      if (change) {
        editor.dispatch({ changes: change })
      }
    }
  }, [
    current,
    revision,
    writable,
    extensions,
    compartments,
    settings.autoComplete,
    settings.syntaxValidation,
    settings.keybindings,
    settings.mode,
  ])

  // Jumping to a line, asked for by the outline and by search. An event
  // rather than a call: neither of them holds the CodeMirror view, and giving
  // one to them would tie three components together for one jump.
  useEffect(() => {
    const jump = (event: Event) => {
      const editor = viewRef.current
      const detail = (event as CustomEvent<{ line: number }>).detail
      if (!editor || !detail) {
        return
      }
      const total = editor.state.doc.lines
      const line = editor.state.doc.line(
        Math.min(Math.max(detail.line + 1, 1), total)
      )
      editor.dispatch({
        selection: { anchor: line.from },
        effects: EditorView.scrollIntoView(line.from, { y: 'center' }),
      })
      editor.focus()
    }
    window.addEventListener('ide:goto-line', jump)
    return () => window.removeEventListener('ide:goto-line', jump)
  }, [])

  // The other half of the jump: where the cursor is, so the PDF pane can ask
  // the compiler where that ended up. Only this component knows the line, and
  // only the PDF pane can scroll to the answer, so the two meet on an event.
  useEffect(() => {
    const locate = () => {
      const editor = viewRef.current
      if (!editor || !current) {
        return
      }
      const line = editor.state.doc.lineAt(editor.state.selection.main.head)
      window.dispatchEvent(
        new CustomEvent('ide:show-in-pdf', {
          detail: { file: current.path, line: line.number },
        })
      )
    }
    window.addEventListener('ide:sync-to-pdf', locate)
    return () => window.removeEventListener('ide:sync-to-pdf', locate)
  }, [current])

  // What the menus ask for. Events rather than calls: the menu bar does not
  // hold the CodeMirror view, and handing it one would tie the two together
  // for what is a handful of commands.
  useEffect(() => {
    const view = () => viewRef.current
    const insertAtCursor = (snippet: string) => {
      const editor = view()
      if (!editor) {
        return
      }
      const at = editor.state.selection.main
      editor.dispatch({
        changes: { from: at.from, to: at.to, insert: snippet },
        selection: { anchor: at.from + snippet.length },
      })
      editor.focus()
    }

    const handlers: Record<string, (event: Event) => void> = {
      'ide:insert': event => {
        const detail = (event as CustomEvent<{ snippet: string }>).detail
        if (detail) {
          insertAtCursor(detail.snippet)
        }
      },
      // A wrap puts the selection inside the command's braces, so selecting a
      // word and choosing Bold does what it looks like it will.
      'ide:wrap': event => {
        const detail = (event as CustomEvent<{ snippet: string }>).detail
        const editor = view()
        if (!detail || !editor) {
          return
        }
        const at = editor.state.selection.main
        const selected = editor.state.sliceDoc(at.from, at.to)
        const brace = detail.snippet.lastIndexOf('{')
        const text =
          detail.snippet.slice(0, brace + 1) + selected + detail.snippet.slice(brace + 1)
        editor.dispatch({
          changes: { from: at.from, to: at.to, insert: text },
          selection: { anchor: at.from + brace + 1 + selected.length },
        })
        editor.focus()
      },
      'ide:undo': () => {
        const editor = view()
        if (editor) {
          undo(editor)
          editor.focus()
        }
      },
      'ide:redo': () => {
        const editor = view()
        if (editor) {
          redo(editor)
          editor.focus()
        }
      },
      'ide:find': () => {
        const editor = view()
        if (editor) {
          openSearchPanel(editor)
        }
      },
    }

    for (const [name, handler] of Object.entries(handlers)) {
      window.addEventListener(name, handler)
    }
    return () => {
      for (const [name, handler] of Object.entries(handlers)) {
        window.removeEventListener(name, handler)
      }
    }
  }, [])

  // The tracked changes the editor should be showing.
  useEffect(() => {
    const editor = viewRef.current
    if (editor) {
      editor.dispatch({ effects: setTrackedChanges.of(review.ranges.changes) })
    }
  }, [review.ranges])

  // Undoing a suggestion is an ordinary edit, so it is made here rather than
  // asked of the server: it then transforms against whatever anybody else is
  // doing, like any other edit would.
  useEffect(() => {
    const reject = (event: Event) => {
      const editor = viewRef.current
      const detail = (event as CustomEvent<{ changes: Parameters<typeof rejectionChanges>[0] }>).detail
      if (!editor || !detail) {
        return
      }
      const changes = rejectionChanges(detail.changes, editor.state.doc.length)
      if (changes.length > 0) {
        editor.dispatch({ changes })
      }
    }
    window.addEventListener('ide:reject-changes', reject)
    return () => window.removeEventListener('ide:reject-changes', reject)
  }, [])

  // Settings that can change without a reload.
  useEffect(() => {
    const editor = viewRef.current
    if (!editor) {
      return
    }
    editor.dispatch({
      effects: [
        compartments.editable.reconfigure(EditorView.editable.of(writable)),
        compartments.completion.reconfigure(
          settings.autoComplete ? autocompletion({ override: [latexCompletions] }) : []
        ),
        compartments.linting.reconfigure(
          settings.syntaxValidation ? linter(latexDiagnostics) : []
        ),
        compartments.keys.reconfigure(settings.keybindings === 'vim' ? vim() : []),
        compartments.visual.reconfigure(
          settings.mode === 'visual' ? visualMode() : []
        ),
      ],
    })
  }, [
    writable,
    settings.autoComplete,
    settings.syntaxValidation,
    settings.keybindings,
    settings.mode,
    compartments,
  ])

  // The container is always rendered, even with nothing open. CodeMirror is
  // created once against it, and it cannot be created against an element that
  // is not there yet -- returning a placeholder instead of this div would mean
  // the view is never built, because the effect that builds it does not run
  // again when the div finally appears.
  return (
    <div className="relative h-full min-h-0">
      <div
        ref={host}
        className="h-full min-h-0 overflow-hidden"
        data-testid="source-editor"
      />
      {!current ? (
        <div className="absolute inset-0 flex items-center justify-center bg-background px-6 text-center text-sm text-default-400">
          {loading ? 'Opening…' : 'Choose a file from the tree to start editing.'}
        </div>
      ) : null}
    </div>
  )
}
