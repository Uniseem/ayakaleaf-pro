import type { OutputFile, FileEntry } from '@/lib/editor'
import HumanReadableLogs from '../human-readable-logs'
import BibLogParser, { type BibLogEntry } from '../log-parser/bib-log-parser'
import type { LogEntries, LogEntry, PDFFile } from './types'
import type { Annotation } from '@/features/source-editor/extensions/annotations'
import { debugConsole } from '@/lib/debug'

// Warnings that may disappear after a second LaTeX pass
const TRANSIENT_WARNING_REGEX = /^(Reference|Citation).+undefined on input line/

const MAX_LOG_SIZE = 1024 * 1024 // 1MB
const MAX_BIB_LOG_SIZE_PER_FILE = MAX_LOG_SIZE

/** The PDF among the output files, with the addresses the viewer needs. */
export function handleOutputFiles(outputFiles: Map<string, OutputFile>): PDFFile | null {
  const outputFile = outputFiles.get('output.pdf')
  if (!outputFile) return null

  return {
    ...outputFile,
    type: 'pdf',
    size: outputFile.size ?? 0,
    downloadURL: outputFile.url,
    // the address the viewer reads from, and the one a download link uses
    pdfUrl: outputFile.url,
    pdfDownloadUrl: outputFile.url,
  }
}

let nextEntryId = 1

function generateEntryKey(): string {
  return 'compile-log-entry-' + nextEntryId++
}

type LogResult = {
  log: string | null
  logEntries: LogEntries
}

type PartialEntries = {
  errors?: (Partial<LogEntry> | BibLogEntry)[]
  warnings?: (Partial<LogEntry> | BibLogEntry)[]
  typesetting?: (Partial<LogEntry> | BibLogEntry)[]
  all?: (Partial<LogEntry> | BibLogEntry)[]
}

/** Reads the log files a compile produced and turns them into entries. */
export async function handleLogFiles(
  outputFiles: Map<string, OutputFile>,
  status: string,
  stopOnFirstError: boolean,
  signal: AbortSignal
): Promise<LogResult> {
  const result: LogResult = {
    log: null,
    logEntries: {
      all: [],
      errors: [],
      warnings: [],
      typesetting: [],
    },
  }

  function accumulateResults(newEntries: PartialEntries, type?: string) {
    for (const key of Object.keys(result.logEntries) as Array<keyof LogEntries>) {
      const entries = newEntries[key]
      if (entries) {
        for (const entry of entries as LogEntry[]) {
          if (type) {
            entry.type = type
          }
          if (entry.file) {
            entry.file = normalizeFilePath(entry.file)
          }
          entry.key = generateEntryKey()
        }
        result.logEntries[key].push(...(entries as LogEntry[]))
      }
    }
  }

  const logFile = outputFiles.get('output.log')

  if (logFile) {
    result.log = await fetchFileWithSizeLimit(logFile.url, signal, MAX_LOG_SIZE)
    try {
      let { errors, warnings, typesetting } = HumanReadableLogs.parse(result.log, {
        ignoreDuplicates: true,
      })

      if (status === 'stopped-on-first-error' || stopOnFirstError) {
        // Hide warnings that could disappear after a second pass
        warnings = warnings.filter(warning => !isTransientWarning(warning))
      }

      accumulateResults({ errors, warnings, typesetting })
    } catch (e) {
      debugConsole.warn(e) // ignore failure to parse the log file, but log a warning
    }
  }

  const blgFiles: OutputFile[] = []

  for (const [filename, file] of outputFiles) {
    if (filename.endsWith('.blg')) {
      blgFiles.push(file)
    }
  }
  for (const blgFile of blgFiles) {
    const log = await fetchFileWithSizeLimit(blgFile.url, signal, MAX_BIB_LOG_SIZE_PER_FILE)
    try {
      const { errors, warnings } = new BibLogParser(log, {
        maxErrors: 100,
      }).parse()
      accumulateResults({ errors, warnings }, 'BibTeX:')
    } catch {
      // BibLog parsing errors are ignored
    }
  }

  result.logEntries.all = [...result.logEntries.errors, ...result.logEntries.warnings, ...result.logEntries.typesetting]

  return result
}

/**
 * The log entries as marks in the editor, keyed by the document they
 * belong to.
 */
export function buildLogEntryAnnotations(
  entries: LogEntry[],
  files: FileEntry[],
  rootDocId?: string | null
): Record<string, Annotation[]> {
  const rootDoc = rootDocId ? files.find(entry => entry.id === rootDocId) : undefined
  const rootDocDirname = rootDoc ? dirnameOf(rootDoc.path) : null

  const byPath = new Map<string, FileEntry>()
  for (const entry of files) {
    if (entry.kind !== 'folder') {
      byPath.set(entry.path.replace(/^\//, ''), entry)
    }
  }

  const logEntryAnnotations: Record<string, Annotation[]> = {}
  const seenLine: Record<number, boolean> = {}

  for (const entry of entries) {
    if (entry.file) {
      entry.file = normalizeFilePath(entry.file, rootDocDirname)

      const entity = byPath.get(entry.file)

      if (entity) {
        if (!(entity.id in logEntryAnnotations)) {
          logEntryAnnotations[entity.id] = []
        }

        const annotation: Annotation = {
          id: entry.key,
          entryIndex: logEntryAnnotations[entity.id]!.length, // used for maintaining the order of items on the same line
          row: (entry.line || 1) - 1,
          column: 0,
          type: entry.level === 'error' ? 'error' : 'warning',
          text: entry.message ?? '',
          source: 'compile',
          ruleId: entry.ruleId,
          command: entry.command,
        }

        // set firstOnLine for the first non-typesetting annotation on a line
        if (entry.level !== 'typesetting') {
          if (!seenLine[entry.line || 0]) {
            annotation.firstOnLine = true
            seenLine[entry.line || 0] = true
          }
        }

        logEntryAnnotations[entity.id]!.push(annotation)
      }
    }
  }

  return logEntryAnnotations
}

const dirnameOf = (path: string) => {
  const trimmed = path.replace(/^\//, '')
  const index = trimmed.lastIndexOf('/')
  return index === -1 ? '' : trimmed.slice(0, index)
}

export function normalizeFilePath(path: string, rootDocDirname?: string | null): string {
  path = path.replace(/\/\//g, '/')
  path = path.replace(/^.*\/compiles\/[0-9a-f]{24}(-[0-9a-f]{24})?\/(\.\/)?/, '')
  path = path.replace(/^\/compile\//, '')

  if (rootDocDirname) {
    path = path.replace(/^\.\//, rootDocDirname + '/')
  }
  path = path.replace(/^\.\//, '')

  return path
}

function isTransientWarning(warning: LogEntry): boolean {
  return TRANSIENT_WARNING_REGEX.test(warning.message || '')
}

async function fetchFileWithSizeLimit(url: string, signal: AbortSignal, maxSize: number): Promise<string> {
  let result = ''
  try {
    const abortController = new AbortController()
    // abort fetching the log file if the main signal is aborted
    signal.addEventListener('abort', () => {
      abortController.abort()
    })

    const response = await fetch(url, {
      signal: abortController.signal,
      credentials: 'include',
    })

    if (!response.ok) {
      throw new Error('Failed to fetch log file')
    }

    const reader = response.body?.pipeThrough(new TextDecoderStream()).getReader()
    if (reader) {
      for (;;) {
        const { done, value } = await reader.read()
        if (done) {
          break
        }
        result += value
        if (result.length > maxSize) {
          abortController.abort()
        }
      }
    }
  } catch (e) {
    debugConsole.warn(e) // ignore failure to fetch the log file, but log a warning
  }
  return result
}
