/**
 * Suggestions while typing LaTeX.
 *
 * Context decides what is offered, because a list of every command at every
 * position is the same as no list at all. After a backslash it is commands;
 * inside `\ref{` it is the labels this document actually defines; inside
 * `\usepackage{` it is packages; inside `\begin{` it is environments, and
 * choosing one writes the matching `\end` too.
 *
 * The document's own labels, citations and definitions come from the scan in
 * analyse.ts, so a label typed a minute ago is suggested a minute later
 * without anything having to be told about it.
 */

import type { Completion, CompletionContext, CompletionResult } from '@codemirror/autocomplete'
import { snippetCompletion } from '@codemirror/autocomplete'
import { analyse } from './analyse'
import {
  CLASSES,
  COMMANDS,
  ENVIRONMENTS,
  PACKAGES,
  type Command,
} from './latex-knowledge'

const BACKSLASH = String.fromCharCode(92)

/** Turns `frac{#}{}` into CodeMirror's `frac{${}}{}` snippet form. */
function toSnippet(body: string): string {
  // `#` marks where the cursor goes; CodeMirror wants `${}` for that.
  const marked = body.includes('#') ? body : body + '#'
  return marked.replace('#', '${}')
}

function commandCompletion(command: Command): Completion {
  const label = BACKSLASH + command.name
  if (!command.snippet) {
    return {
      label,
      type: 'keyword',
      detail: command.detail,
      boost: command.boost,
    }
  }
  return snippetCompletion(BACKSLASH + toSnippet(command.snippet), {
    label,
    type: 'function',
    detail: command.detail,
    boost: command.boost,
  })
}

/** Built once: the list does not change, and rebuilding it per keystroke shows. */
const COMMAND_COMPLETIONS = COMMANDS.map(commandCompletion)

/**
 * What kind of argument the cursor is sitting inside, if any.
 *
 * Looks backwards for an unclosed `{` and the command in front of it. Cheap,
 * and wrong only for an argument spanning more than the 200 characters it
 * looks back over -- which a `\ref{` never is.
 */
function argumentContext(
  before: string
): { command: string; typed: string; from: number } | null {
  const window = before.slice(-200)
  const open = window.lastIndexOf('{')
  if (open === -1) {
    return null
  }
  const inside = window.slice(open + 1)
  // A closing brace or a newline means that group is already finished.
  if (inside.includes('}') || inside.includes('\n')) {
    return null
  }
  const head = window.slice(0, open)
  // The command, allowing an optional [...] between it and the brace.
  const match = head.match(/\\([a-zA-Z@]+)\s*(?:\[[^\]]*\])?\s*$/)
  if (!match) {
    return null
  }
  return {
    command: match[1] ?? '',
    typed: inside,
    from: before.length - inside.length,
  }
}

const REFERENCE_COMMANDS = new Set([
  'ref', 'eqref', 'pageref', 'autoref', 'nameref', 'cref', 'Cref', 'vref',
])
const CITATION_COMMANDS = new Set([
  'cite', 'citep', 'citet', 'citeauthor', 'citeyear', 'parencite',
  'textcite', 'autocite', 'nocite',
])

export function latexCompletions(
  context: CompletionContext
): CompletionResult | null {
  const source = context.state.doc.toString()
  const before = source.slice(0, context.pos)

  const argument = argumentContext(before)
  if (argument) {
    const { command, typed, from } = argument

    if (command === 'begin' || command === 'end') {
      const document = analyse(source)
      // Environments this document already uses come first: a document with
      // twenty `theorem` environments almost certainly wants another.
      const used = new Set(document.environments.map(each => each.name))
      const options: Completion[] = ENVIRONMENTS.map(environment => ({
        label: environment.name,
        type: 'class',
        detail: environment.detail,
        boost: (environment.boost ?? 0) + (used.has(environment.name) ? 5 : 0),
      }))
      for (const name of used) {
        if (!ENVIRONMENTS.some(each => each.name === name)) {
          options.push({ label: name, type: 'class', detail: 'in this document', boost: 4 })
        }
      }
      return { from, options, validFor: /^[\w*-]*$/ }
    }

    if (REFERENCE_COMMANDS.has(command)) {
      const document = analyse(source)
      return {
        from,
        options: document.labels.map(label => ({
          label: label.name,
          type: 'variable',
          detail: `line ${label.line + 1}`,
        })),
        validFor: /^[\w:.-]*$/,
      }
    }

    if (CITATION_COMMANDS.has(command)) {
      const document = analyse(source)
      // Keys already cited elsewhere. A .bib file's keys would be better and
      // need the file's contents, which the editor does not have here.
      const keys = [...new Set(document.citations.map(each => each.key))]
      return {
        from,
        options: keys.map(key => ({ label: key, type: 'variable' })),
        validFor: /^[\w:.-]*$/,
      }
    }

    if (command === 'usepackage' || command === 'RequirePackage') {
      return {
        from,
        options: PACKAGES.map(name => ({ label: name, type: 'namespace' })),
        validFor: /^[\w-]*$/,
      }
    }

    if (command === 'documentclass' || command === 'LoadClass') {
      return {
        from,
        options: CLASSES.map(name => ({ label: name, type: 'namespace' })),
        validFor: /^[\w-]*$/,
      }
    }

    // An argument of something else. Offering commands here is right --
    // \textbf{\emph{...}} is ordinary -- but only once a backslash is typed,
    // which the branch below handles.
    if (!typed.startsWith(BACKSLASH)) {
      return null
    }
  }

  // A command: a backslash and the letters after it.
  const word = context.matchBefore(/\\[a-zA-Z@]*/)
  if (!word || (word.from === word.to && !context.explicit)) {
    return null
  }

  const document = analyse(source)
  const options: Completion[] = [...COMMAND_COMPLETIONS]

  // Commands this document defines for itself.
  for (const definition of document.definitions) {
    options.push({
      label: BACKSLASH + definition.name,
      type: 'function',
      detail: definition.arity > 0 ? `${definition.arity} arguments` : 'defined here',
      boost: 6,
    })
  }

  return { from: word.from, options, validFor: /^\\[a-zA-Z@]*$/ }
}

/**
 * Completing `\begin{x}` by writing the matching `\end{x}`.
 *
 * Run as a separate step after the environment name is chosen, because the
 * name is only known then and the closing line depends on it.
 */
export function environmentBody(name: string): string {
  const known = ENVIRONMENTS.find(each => each.name === name)
  return known?.body ?? '  '
}
