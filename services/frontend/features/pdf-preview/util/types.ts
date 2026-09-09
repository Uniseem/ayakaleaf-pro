import type { ReactNode } from 'react'
import type { OutputFile } from '@/lib/editor'

export type ErrorLevel = 'error' | 'warning' | 'info' | 'typesetting' | 'raw' | 'success'

/** One thing a compile log had to say, as the logs pane shows it. */
export type LogEntry = {
  raw: string
  level: ErrorLevel
  key: string
  file?: string
  column?: number
  line?: number
  ruleId?: string
  message?: string
  content?: string
  type?: string
  messageComponent?: ReactNode
  contentDetails?: string[]
  command?: string
  suppressed?: boolean
}

export type LogEntries = {
  all: LogEntry[]
  errors: LogEntry[]
  warnings: LogEntry[]
  typesetting: LogEntry[]
}

export type SourceLocation = {
  file?: string
  // `line` should be either a number or null (i.e. not required), but
  // sometimes it is an empty string (from BibTeX errors).
  line?: number | string | null
  column?: number
}

/** An output file, with what the pane works out about it. */
export type PdfFileData = OutputFile & {
  main?: boolean
  downloadURL?: string
}

type PdfFileArchiveData = PdfFileData & { fileCount: number }

export type PdfFileDataList = {
  top: PdfFileData[]
  other: PdfFileData[]
  archive?: PdfFileArchiveData
}

/** The PDF among the output files, with the addresses the viewer needs. */
export type PDFFile = PdfFileData & {
  pdfUrl: string
  pdfDownloadUrl: string
  size: number
}

export type HighlightData = {
  page: number
  h: number
  v: number
  width: number
  height: number
}

/** Where the PDF is scrolled to: a page, an offset into it, and its size. */
export type PdfScrollPosition = {
  page: number
  offset: { top: number; left: number }
  pageSize?: { height: number; width: number }
}

export type CompileOptions = {
  draft?: boolean
  stopOnFirstError?: boolean
  isAutoCompileOnLoad?: boolean
  isAutoCompileOnChange?: boolean
  rootDocId?: string
  incremental?: boolean
}

export type ValidationProblems = {
  sizeCheck?: { resources: { path: string; kbSize: number }[] }
  conflictedPaths?: { path: string }[]
  mainFile?: boolean
}
