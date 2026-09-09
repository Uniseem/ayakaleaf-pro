/**
 * Reading a LaTeX document for the things the editor needs to know about it.
 *
 * Its own labels, so \ref can suggest them. Its own \newcommand definitions,
 * so completion offers those too. Its sections, for the outline. Where each
 * environment opens and closes, for folding and for saying which one was never
 * closed.
 *
 * This is a scan, not a parse: it works line by line and does not build a
 * tree. That is deliberate -- it runs on every edit of a document that may be
 * ten thousand lines, and it has to survive a document that is, at the moment
 * it runs, syntactically broken. Every real editor session spends most of its
 * time in that state.
 */

import { MATH_ENVIRONMENTS, SECTION_COMMANDS, VERBATIM_ENVIRONMENTS } from './latex-knowledge'

export type Label = { name: string; line: number }
export type Citation = { key: string; line: number }
export type Definition = { name: string; line: number; arity: number }

export type Section = {
  title: string
  /** 0 for part, 1 for chapter, and so on down. */
  level: number
  line: number
  from: number
}

export type EnvironmentSpan = {
  name: string
  /** Character offset of the \begin. */
  from: number
  /** Character offset of the end of \end, or null when it was never closed. */
  to: number | null
  fromLine: number
  toLine: number | null
}

export type Analysis = {
  labels: Label[]
  citations: Citation[]
  definitions: Definition[]
  sections: Section[]
  environments: EnvironmentSpan[]
  /** Environments opened and never closed. */
  unclosed: EnvironmentSpan[]
  /** \end without a matching \begin, as offsets. */
  unopened: { name: string; from: number; line: number }[]
  /** Whether the document has a \begin{document}. */
  hasDocument: boolean
}

const BEGIN = /\\begin\s*\{([^}]*)\}/g
const END = /\\end\s*\{([^}]*)\}/g
const LABEL = /\\label\s*\{([^}]*)\}/g
const CITE = /\\(?:cite|citep|citet|citeauthor|citeyear|parencite|textcite|autocite)\s*(?:\[[^\]]*\])*\s*\{([^}]*)\}/g
const DEFINE = /\\(?:re)?newcommand\s*\*?\s*\{?\\([a-zA-Z@]+)\}?\s*(?:\[(\d+)\])?/g
const SECTION = /\\(part|chapter|section|subsection|subsubsection|paragraph|subparagraph)\s*\*?\s*(?:\[[^\]]*\])?\s*\{/

/** Strips comments, so nothing inside one is taken as real. */
function withoutComment(line: string): string {
  let escaped = false
  for (let i = 0; i < line.length; i++) {
    const character = line[i]
    if (escaped) {
      escaped = false
      continue
    }
    if (character === '\\') {
      escaped = true
      continue
    }
    if (character === '%') {
      return line.slice(0, i)
    }
  }
  return line
}

/**
 * Reads the braced group starting at `open`, respecting nesting.
 *
 * Needed for section titles, which routinely contain braces of their own --
 * `\section{The \texttt{foo} problem}` -- and which a non-greedy `[^}]*` would
 * cut in half.
 */
function readGroup(text: string, open: number): { body: string; end: number } | null {
  if (text[open] !== '{') {
    return null
  }
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
        return { body: text.slice(open + 1, i), end: i + 1 }
      }
    }
  }
  return null
}

/** Removes the commands from a section title, leaving what it reads as. */
function plainTitle(body: string): string {
  return body
    .replace(/\\[a-zA-Z@]+\s*/g, '')
    .replace(/[{}]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

export function analyse(source: string): Analysis {
  const lines = source.split('\n')
  const labels: Label[] = []
  const citations: Citation[] = []
  const definitions: Definition[] = []
  const sections: Section[] = []
  const environments: EnvironmentSpan[] = []
  const unopened: { name: string; from: number; line: number }[] = []
  const open: EnvironmentSpan[] = []
  let hasDocument = false

  // Character offset of the start of the current line, kept as we go so that
  // matches can be reported as document positions without re-measuring.
  let offset = 0
  // Set while inside verbatim, where nothing means what it says.
  let verbatim: string | null = null

  for (let number = 0; number < lines.length; number++) {
    const raw = lines[number] ?? ''

    if (verbatim) {
      // Only the matching \end gets us out.
      const closing = new RegExp(`\\\\end\\s*\\{${escapeRegExp(verbatim)}\\}`)
      const found = raw.match(closing)
      if (found) {
        const span = open.pop()
        if (span) {
          span.to = offset + (found.index ?? 0) + found[0].length
          span.toLine = number
          environments.push(span)
        }
        verbatim = null
      }
      offset += raw.length + 1
      continue
    }

    const line = withoutComment(raw)

    for (const match of line.matchAll(BEGIN)) {
      const name = (match[1] ?? '').trim()
      const span: EnvironmentSpan = {
        name,
        from: offset + (match.index ?? 0),
        to: null,
        fromLine: number,
        toLine: null,
      }
      open.push(span)
      if (name === 'document') {
        hasDocument = true
      }
      if (VERBATIM_ENVIRONMENTS.has(name)) {
        verbatim = name
        break
      }
    }

    if (!verbatim) {
      for (const match of line.matchAll(END)) {
        const name = (match[1] ?? '').trim()
        // Find the innermost matching open. A mismatched \end closes nothing,
        // which is what makes "\begin{a} \end{b}" report both problems rather
        // than silently pairing them.
        const at = [...open].reverse().findIndex(span => span.name === name)
        if (at === -1) {
          unopened.push({
            name,
            from: offset + (match.index ?? 0),
            line: number,
          })
          continue
        }
        const index = open.length - 1 - at
        const span = open[index]
        if (span) {
          span.to = offset + (match.index ?? 0) + match[0].length
          span.toLine = number
          environments.push(span)
        }
        open.splice(index, 1)
      }

      for (const match of line.matchAll(LABEL)) {
        const name = (match[1] ?? '').trim()
        if (name) {
          labels.push({ name, line: number })
        }
      }

      for (const match of line.matchAll(CITE)) {
        for (const key of (match[1] ?? '').split(',')) {
          const trimmed = key.trim()
          if (trimmed) {
            citations.push({ key: trimmed, line: number })
          }
        }
      }

      for (const match of line.matchAll(DEFINE)) {
        definitions.push({
          name: match[1] ?? '',
          line: number,
          arity: match[2] ? Number(match[2]) : 0,
        })
      }

      const heading = line.match(SECTION)
      if (heading && heading.index !== undefined) {
        const braceAt = heading.index + heading[0].length - 1
        const group = readGroup(line, braceAt)
        const command = heading[1] ?? ''
        sections.push({
          title: group ? plainTitle(group.body) : '',
          level: SECTION_COMMANDS.indexOf(command),
          line: number,
          from: offset + heading.index,
        })
      }
    }

    offset += raw.length + 1
  }

  return {
    labels,
    citations,
    definitions,
    sections,
    environments,
    unclosed: open,
    unopened,
    hasDocument,
  }
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, (character) => '\\' + character)
}

/** Whether an offset is inside maths, which decides what to suggest there. */
export function inMath(source: string, at: number): boolean {
  const before = source.slice(0, at)

  // An environment that is maths, still open at this point.
  const opened: string[] = []
  for (const match of before.matchAll(/\\(begin|end)\s*\{([^}]*)\}/g)) {
    const name = (match[2] ?? '').trim()
    if (match[1] === 'begin') {
      opened.push(name)
    } else {
      const at2 = opened.lastIndexOf(name)
      if (at2 !== -1) {
        opened.splice(at2, 1)
      }
    }
  }
  if (opened.some(name => MATH_ENVIRONMENTS.has(name))) {
    return true
  }

  // Or an odd number of unescaped dollars, which means one is still open.
  // \[ ... \] is counted the same way.
  let dollars = 0
  let displays = 0
  for (let i = 0; i < before.length; i++) {
    if (before[i] === '\\') {
      const next = before[i + 1]
      if (next === '[') {
        displays++
      } else if (next === ']') {
        displays--
      }
      i++
      continue
    }
    if (before[i] === '$') {
      // $$ is one delimiter, not two.
      if (before[i + 1] === '$') {
        i++
      }
      dollars++
    }
  }
  return displays > 0 || dollars % 2 === 1
}
