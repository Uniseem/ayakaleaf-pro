/**
 * Reading a LaTeX log.
 *
 * TeX reports trouble in a format that predates every convention a parser
 * would like: an error is a line starting with `!` whose detail arrives over
 * the next few lines, a warning may be one line or five, and which file any of
 * it happened in is known only by counting the parentheses TeX prints as it
 * opens and closes files. Nothing is delimited and nothing is escaped.
 *
 * So this is a small state machine over the log, and the rules it encodes are
 * TeX's, not ours. The alternative -- showing the person the raw log -- is
 * what the editor did before it could do this, and it means finding your own
 * error in four thousand lines of font warnings.
 */

/** How wide TeX wraps its log, which is how the wrapping is undone. */
const WRAP_COLUMN = 79

const LATEX_WARNING = /^LaTeX(?:3| Font)? Warning: (.*)$/
const BOX_WARNING = /^(Over|Under)full \\(v|h)box/
const PACKAGE_WARNING = /^((?:Package|Class|Module) \b.+\b Warning:.*)$/
const PACKAGE_NAME = /^(?:Package|Class|Module) (\b.+\b) Warning/
/** `./chapters/one.tex:42: Undefined control sequence.` */
const FILE_LINE_ERROR = /^([./].*):(\d+): (.*)/
const LINE_IN_WARNING = /lines? ([0-9]+)/
/** The line TeX prints to say where in the source it gave up. */
const ERROR_LINE = /l\.([0-9]+)/

export type LogLevel = 'error' | 'warning' | 'typesetting'

/** One thing the log had to say. */
export type LogEntry = {
  level: LogLevel
  message: string
  /** The file it happened in, as TeX named it. */
  file?: string
  /** The line in that file, when TeX said. */
  line?: number
  /** The lines beneath the message, for an error that had detail. */
  content?: string
  /** Everything this entry was made from, used to tell duplicates apart. */
  raw: string
}

/** A file TeX opened, and what it opened from inside it. */
export type LogFile = {
  path: string
  files: LogFile[]
}

export type ParsedLog = {
  all: LogEntry[]
  errors: LogEntry[]
  warnings: LogEntry[]
  typesetting: LogEntry[]
  /** The tree of files the run went through. */
  files: LogFile[]
}

/**
 * The log as a list of lines, with TeX's wrapping undone.
 *
 * A line of exactly the wrap column is a line that was cut, so it is joined to
 * the next one -- unless it ends in an ellipsis, which is TeX saying it
 * truncated deliberately, or the next line opens an error, which must stay a
 * line of its own or the state machine will not see it.
 */
class LogLines {
  private readonly lines: string[]
  private row = 0

  constructor(text: string) {
    const raw = text.replace(/\r\n|\r/g, '\n').split('\n')
    this.lines = raw.length > 0 ? [raw[0] ?? ''] : []
    for (let i = 1; i < raw.length; i++) {
      const previous = raw[i - 1] ?? ''
      const current = raw[i] ?? ''
      const wrapped =
        previous.length === WRAP_COLUMN &&
        previous.slice(-3) !== '...' &&
        current.charAt(0) !== '!'
      if (wrapped) {
        this.lines[this.lines.length - 1] += current
      } else {
        this.lines.push(current)
      }
    }
  }

  next(): string | null {
    this.row++
    return this.row < this.lines.length ? (this.lines[this.row] ?? '') : null
  }

  back() {
    this.row--
  }

  /**
   * Takes lines until one matches, and returns them.
   *
   * `stopAtError` leaves the next error where it is: an error's detail runs
   * until a blank line, but a log that never gets one would otherwise swallow
   * every error after it into the first.
   */
  until(match: RegExp, stopAtError = false): string[] {
    const taken: string[] = []
    for (;;) {
      const line = this.next()
      if (line === null) {
        break
      }
      if (stopAtError && /^! /.test(line)) {
        this.back()
        break
      }
      taken.push(line)
      if (match.test(line)) {
        break
      }
    }
    return taken
  }

  untilBlank(stopAtError = false): string[] {
    return this.until(/^ *$/, stopAtError)
  }
}

type Options = {
  /** Drop entries whose text has already been reported. */
  ignoreDuplicates?: boolean
}

class Parser {
  private readonly log: LogLines
  private readonly entries: LogEntry[] = []
  /** The files currently open, innermost last. */
  private readonly stack: LogFile[] = []
  private readonly root: LogFile[] = []
  private children: LogFile[] = this.root
  /** Parentheses that were not a file, so the closing one is not a file either. */
  private depth = 0
  private line = ''
  private file: string | undefined

  constructor(text: string) {
    this.log = new LogLines(text)
  }

  parse(options: Options): ParsedLog {
    for (;;) {
      const next = this.log.next()
      if (next === null) {
        break
      }
      this.line = next

      const error = this.startOfError()
      if (error) {
        this.finishError(error)
      } else if (this.line.startsWith('Runaway argument')) {
        this.runawayArgument()
      } else if (LATEX_WARNING.test(this.line)) {
        this.singleLineWarning()
      } else if (BOX_WARNING.test(this.line)) {
        this.boxWarning()
      } else if (PACKAGE_WARNING.test(this.line)) {
        this.packageWarning()
      } else {
        this.followFiles()
      }
    }
    return this.collect(options)
  }

  /** An error's first line, in either of the two forms TeX writes. */
  private startOfError(): LogEntry | null {
    // `! Undefined control sequence.` -- but not the summary TeX prints at the
    // very end, which is not an error of its own.
    if (
      this.line.startsWith('!') &&
      this.line !== '!  ==> Fatal error occurred, no output PDF file produced!'
    ) {
      return {
        level: 'error',
        message: this.line.slice(2),
        file: this.file,
        content: '',
        raw: this.line + '\n',
      }
    }
    // `./main.tex:12: Undefined control sequence.`, which -file-line-error asks
    // for and which says where it happened without any counting.
    const located = this.line.match(FILE_LINE_ERROR)
    if (located) {
      return {
        level: 'error',
        message: located[3] ?? '',
        file: located[1],
        line: Number(located[2]),
        content: '',
        raw: this.line + '\n',
      }
    }
    return null
  }

  /**
   * Collects the detail under an error.
   *
   * TeX prints the offending input, then a line `l.42 ...` naming where it
   * was, then context. Two passes: up to that line, then up to the blank line
   * that ends the report.
   */
  private finishError(error: LogEntry) {
    const upToLine = this.log.until(/^l\.[0-9]+/, false)
    const afterwards = this.log.untilBlank(true)
    error.content = [upToLine.join('\n'), afterwards.join('\n')].join('\n')
    error.raw += error.content

    if (error.line === undefined) {
      const found = error.raw.match(ERROR_LINE)
      if (found) {
        error.line = Number(found[1])
      }
    }
    this.entries.push(error)
  }

  /** `Runaway argument?` is an error whose detail is two paragraphs. */
  private runawayArgument() {
    const entry: LogEntry = {
      level: 'error',
      message: this.line,
      file: this.file,
      content: '',
      raw: this.line + '\n',
    }
    entry.content = [
      this.log.untilBlank().join('\n'),
      this.log.untilBlank().join('\n'),
    ].join('\n')
    entry.raw += entry.content
    const found = entry.raw.match(ERROR_LINE)
    if (found) {
      entry.line = Number(found[1])
    }
    this.entries.push(entry)
  }

  private singleLineWarning() {
    const found = this.line.match(LATEX_WARNING)
    if (!found) {
      return
    }
    const message = found[1] ?? ''
    const where = message.match(LINE_IN_WARNING)
    this.entries.push({
      level: 'warning',
      message,
      file: this.file,
      line: where ? Number(where[1]) : undefined,
      raw: message,
    })
  }

  /** An overfull or underfull box, which is a complaint about spacing. */
  private boxWarning() {
    const where = this.line.match(LINE_IN_WARNING)
    this.entries.push({
      level: 'typesetting',
      message: this.line,
      file: this.file,
      line: where ? Number(where[1]) : undefined,
      raw: this.line,
    })
  }

  /**
   * A package's warning, which runs until a blank line.
   *
   * Every continuation line repeats the package's name in parentheses, so that
   * prefix is stripped -- otherwise the message reads
   * "(hyperref) (hyperref) (hyperref)".
   */
  private packageWarning() {
    const first = this.line.match(PACKAGE_WARNING)
    const named = this.line.match(PACKAGE_NAME)
    if (!first || !named) {
      return
    }
    const parts = [first[1] ?? '']
    let where = this.line.match(LINE_IN_WARNING)
    let line = where ? Number(where[1]) : undefined

    const prefix = new RegExp(`(?:\\(${escapeRegExp(named[1] ?? '')}\\))*[\\s]*(.*)`, 'i')
    for (;;) {
      const next = this.log.next()
      if (!next) {
        break
      }
      this.line = next
      where = this.line.match(LINE_IN_WARNING)
      if (where) {
        line = Number(where[1])
      }
      const continued = this.line.match(prefix)
      parts.push(continued ? (continued[1] ?? '') : this.line)
    }

    const message = parts.join(' ')
    this.entries.push({
      level: 'warning',
      message,
      file: this.file,
      line,
      raw: message,
    })
  }

  /**
   * Tracks which file the log is inside.
   *
   * TeX writes `(./chapter.tex` when it opens a file and `)` when it closes
   * one, in amongst everything else on the line. Parentheses that are not a
   * file are counted so that their closing half does not pop a real file off
   * the stack.
   */
  private followFiles() {
    for (;;) {
      const at = this.line.search(/[()]/)
      if (at === -1) {
        return
      }
      const token = this.line[at]
      this.line = this.line.slice(at + 1)

      if (token === '(') {
        const path = this.takeFilePath()
        if (path) {
          this.file = path
          const opened: LogFile = { path, files: [] }
          this.stack.push(opened)
          this.children.push(opened)
          this.children = opened.files
        } else {
          this.depth++
        }
      } else if (this.depth > 0) {
        this.depth--
      } else if (this.stack.length > 1) {
        this.stack.pop()
        const enclosing = this.stack[this.stack.length - 1]
        if (enclosing) {
          this.file = enclosing.path
          this.children = enclosing.files
        }
      }
    }
  }

  /**
   * Reads a file path off the front of the line, if one is there.
   *
   * There is no way to be sure: a path is not quoted and may contain spaces.
   * The rule is that it must contain a slash before any bracket or backslash,
   * and a space only ends it when what came before looks like a filename or
   * what follows opens something else.
   */
  private takeFilePath(): string | null {
    if (!/^\/?([^ ()\\]+\/)+/.test(this.line)) {
      return null
    }
    let end = this.line.search(/[ ()\\]/)
    while (end !== -1 && this.line[end] === ' ') {
      const sofar = this.line.slice(0, end)
      if (/\.\w+$/.test(sofar)) {
        break
      }
      const rest = this.line.slice(end + 1)
      if (/^\s*["()[\]]/.test(rest)) {
        break
      }
      const further = rest.search(/[ "()[\]]/)
      if (further === -1) {
        end = -1
      } else {
        end += further + 1
      }
    }
    if (end === -1) {
      const whole = this.line
      this.line = ''
      return whole
    }
    const path = this.line.slice(0, end)
    this.line = this.line.slice(end)
    return path
  }

  private collect(options: Options): ParsedLog {
    const all: LogEntry[] = []
    const byLevel: Record<LogLevel, LogEntry[]> = {
      error: [],
      warning: [],
      typesetting: [],
    }
    const seen = new Set<string>()

    for (const entry of this.entries) {
      if (options.ignoreDuplicates && seen.has(entry.raw)) {
        continue
      }
      seen.add(entry.raw)
      byLevel[entry.level].push(entry)
      all.push(entry)
    }

    return {
      all,
      errors: byLevel.error,
      warnings: byLevel.warning,
      typesetting: byLevel.typesetting,
      files: this.root,
    }
  }
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/** Reads a LaTeX log into the things it reported. */
export function parseLatexLog(text: string, options: Options = {}): ParsedLog {
  return new Parser(text).parse(options)
}
