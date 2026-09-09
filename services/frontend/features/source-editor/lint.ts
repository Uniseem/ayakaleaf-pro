/**
 * The mistakes the editor can see without compiling.
 *
 * Only ones that are certainly wrong, and only ones TeX would report badly.
 * An unclosed environment is worth flagging because TeX's own message for it
 * arrives hundreds of lines later pointing at the end of the file; a missing
 * package is not, because guessing at that is how a linter becomes noise
 * somebody turns off.
 */

import type { Diagnostic } from '@codemirror/lint'
import type { EditorView } from '@codemirror/view'
import { analyse } from './analyse'

export function latexDiagnostics(view: EditorView): Diagnostic[] {
  const source = view.state.doc.toString()
  const document = analyse(source)
  const problems: Diagnostic[] = []
  const length = view.state.doc.length

  for (const span of document.unclosed) {
    problems.push({
      from: span.from,
      to: Math.min(span.from + span.name.length + 8, length),
      severity: 'error',
      message: `\\begin{${span.name}} was never closed.`,
      actions: [
        {
          name: 'Close it',
          apply(target) {
            // At the end, on its own line, which is where it almost always
            // belongs and is somewhere the person can see it happen.
            const end = target.state.doc.length
            const line = target.state.doc.lineAt(end)
            const prefix = line.text.trim() === '' ? '' : '\n'
            target.dispatch({
              changes: {
                from: end,
                insert: `${prefix}\\end{${span.name}}\n`,
              },
            })
          },
        },
      ],
    })
  }

  for (const stray of document.unopened) {
    problems.push({
      from: stray.from,
      to: Math.min(stray.from + stray.name.length + 6, length),
      severity: 'error',
      message: `\\end{${stray.name}} has no matching \\begin.`,
    })
  }

  // A document body is required, and its absence produces a TeX error that
  // says "Missing \begin{document}" against a line that is not the problem.
  if (source.trim() && !document.hasDocument && /\\documentclass/.test(source)) {
    problems.push({
      from: 0,
      to: Math.min(20, length),
      severity: 'warning',
      message: 'This file has a \\documentclass but no \\begin{document}.',
    })
  }

  return problems
}
