'use client'

/**
 * Compiling, and what came back.
 *
 * A compile is started from four places -- the button, a keystroke, the
 * automatic recompile, and the "try again" in an error -- and its result is
 * read by three more: the PDF pane, the log pane, and the gutter marks in the
 * editor. Holding it here is what lets all of them agree on whether a compile
 * is running without passing a flag through every component in between.
 *
 * The log is parsed rather than shown: see log-parser.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import {
  compileProject,
  logIn,
  pdfIn,
  stopCompile,
  type CompileResult,
} from '@/lib/editor'
import { messageFor } from '@/lib/api'
import { usePersistedState } from '@/lib/hooks'
import { parseLatexLog, type LogEntry } from '@/features/pdf-preview/log-parser'
import useCompileTriggers from '@/features/pdf-preview/use-compile-triggers'
import { useProject } from './project-context'

/** How long after the last edit an automatic compile runs. */
const AUTO_COMPILE_DELAY = 3000

export type CompileValue = {
  compiling: boolean
  /** Set when the compile could not be run at all, as opposed to failing. */
  error: string | null
  /** What the last compile said about itself: success, failure, timeout. */
  status: string | null

  /** Where the PDF is, or null when the last compile made none. */
  pdfUrl: string | null
  /** True once a compile has finished, so the pane can tell empty from waiting. */
  hasCompiled: boolean

  logEntries: LogEntry[]
  errors: LogEntry[]
  warnings: LogEntry[]
  typesetting: LogEntry[]
  rawLog: string

  /** Whether the source has changed since the PDF was made. */
  stale: boolean
  markEdited: () => void

  autoCompile: boolean
  setAutoCompile: (on: boolean) => void
  draft: boolean
  setDraft: (on: boolean) => void
  stopOnFirstError: boolean
  setStopOnFirstError: (on: boolean) => void

  startCompile: () => void
  stop: () => void
}

const CompileContext = createContext<CompileValue | undefined>(undefined)

export function CompileProvider({ children }: { children: ReactNode }) {
  const { projectId, project } = useProject()

  const [compiling, setCompiling] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<CompileResult | null>(null)
  const [rawLog, setRawLog] = useState('')
  const [hasCompiled, setHasCompiled] = useState(false)
  const [stale, setStale] = useState(false)

  const [autoCompile, setAutoCompile] = usePersistedState(
    'compile.auto',
    true
  )
  const [draft, setDraft] = usePersistedState('compile.draft', false)
  const [stopOnFirstError, setStopOnFirstError] = usePersistedState(
    'compile.stopOnFirstError',
    false
  )

  // The compile in flight, so a second one can replace it rather than race it.
  const running = useRef<AbortController | null>(null)
  // Read inside the debounce, which would otherwise close over a stale copy.
  const options = useRef({ draft, stopOnFirstError, rootDocId: project.rootDocId })
  options.current = { draft, stopOnFirstError, rootDocId: project.rootDocId }

  const startCompile = useCallback(async () => {
    running.current?.abort()
    const controller = new AbortController()
    running.current = controller

    setCompiling(true)
    setError(null)
    try {
      const compiled = await compileProject(
        projectId,
        options.current,
        controller.signal
      )
      if (controller.signal.aborted) {
        return
      }
      setResult(compiled)
      setHasCompiled(true)
      setStale(false)

      // The log is a separate file the compiler wrote; it is fetched rather
      // than returned so that a large log does not sit in the compile
      // response, and so that the PDF can be shown before it arrives.
      const log = logIn(compiled)
      if (log) {
        try {
          const response = await fetch(log.url, { credentials: 'include' })
          setRawLog(response.ok ? await response.text() : '')
        } catch {
          setRawLog('')
        }
      } else {
        setRawLog('')
      }
    } catch (thrown) {
      if (!controller.signal.aborted) {
        setError(messageFor(thrown))
      }
    } finally {
      if (running.current === controller) {
        running.current = null
        setCompiling(false)
      }
    }
  }, [projectId])

  const stop = useCallback(() => {
    running.current?.abort()
    running.current = null
    setCompiling(false)
    void stopCompile(projectId).catch(() => {
      // The compile is already being abandoned on this side; the server
      // failing to hear that is not something to report to the person.
    })
  }, [projectId])

  const markEdited = useCallback(() => setStale(true), [])

  // Ctrl/Cmd+Enter, Ctrl/Cmd+S and the editor's save commands compile
  const startCompileNow = useCallback(() => {
    void startCompile()
  }, [startCompile])
  useCompileTriggers(startCompileNow)

  // The automatic recompile. It waits for a pause rather than compiling on
  // every keystroke, and it does not start one while one is running.
  useEffect(() => {
    if (!autoCompile || !stale || compiling) {
      return
    }
    const timer = setTimeout(() => {
      void startCompile()
    }, AUTO_COMPILE_DELAY)
    return () => clearTimeout(timer)
  }, [autoCompile, stale, compiling, startCompile])

  // Abandon whatever is running when the project is left.
  useEffect(() => () => running.current?.abort(), [])

  const parsed = useMemo(
    () => parseLatexLog(rawLog, { ignoreDuplicates: true }),
    [rawLog]
  )

  const value = useMemo<CompileValue>(() => {
    const pdf = pdfIn(result)
    return {
      compiling,
      error,
      status: result?.status ?? null,
      pdfUrl: pdf?.url ?? null,
      hasCompiled,
      logEntries: parsed.all,
      errors: parsed.errors,
      warnings: parsed.warnings,
      typesetting: parsed.typesetting,
      rawLog,
      stale,
      markEdited,
      autoCompile,
      setAutoCompile,
      draft,
      setDraft,
      stopOnFirstError,
      setStopOnFirstError,
      startCompile: () => void startCompile(),
      stop,
    }
  }, [
    compiling,
    error,
    result,
    hasCompiled,
    parsed,
    rawLog,
    stale,
    markEdited,
    autoCompile,
    setAutoCompile,
    draft,
    setDraft,
    stopOnFirstError,
    setStopOnFirstError,
    startCompile,
    stop,
  ])

  return (
    <CompileContext.Provider value={value}>{children}</CompileContext.Provider>
  )
}

export function useCompile(): CompileValue {
  const value = useContext(CompileContext)
  if (!value) {
    throw new Error('useCompile must be used inside a CompileProvider')
  }
  return value
}
