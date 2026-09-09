'use client'

/**
 * Compiling, and everything the PDF pane knows about the result.
 *
 * From shared/context/local-compile-context. One compile runs at a time;
 * asking for another while one runs queues an automatic one. The response
 * names the output files; the PDF is shown from its address and the logs are
 * fetched and read separately, so a large log never sits in the response
 * and the PDF can appear before the log has been explained.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from 'react'
import { usePersistedState, useAbortController, useEventListener } from '@/lib/hooks'
import type { OutputFile } from '@/lib/editor'
import DocumentCompiler, { type CompileData } from '@/features/pdf-preview/util/compiler'
import { buildLogEntryAnnotations, handleLogFiles, handleOutputFiles } from '@/features/pdf-preview/util/output-files'
import { buildFileList } from '@/features/pdf-preview/util/file-list'
import { usePdfScrollPosition } from '@/features/pdf-preview/hooks/use-pdf-scroll-position'
import useCompileTriggers from '@/features/pdf-preview/use-compile-triggers'
import { useLogEvents } from '@/features/pdf-preview/hooks/use-log-events'
import { useOpenDocAtLine } from '@/features/ide/hooks/use-open-doc-at-line'
import type {
  CompileOptions,
  HighlightData,
  LogEntries,
  PDFFile,
  PdfFileDataList,
  PdfScrollPosition,
  SourceLocation,
  ValidationProblems,
} from '@/features/pdf-preview/util/types'
import type { Annotation } from '@/features/source-editor/extensions/annotations'
import { useProject } from './project-context'
import { useEditor } from './editor-context'
import { useLayout } from './layout-context'
import { useSettings } from './settings-context'
import { useSite } from './site-context'
import { setWikiEnabled } from '@/features/pdf-preview/human-readable-logs/hints'

export type CompileValue = {
  animateCompileDropdownArrow: boolean
  autoCompile: boolean
  clearCache: () => Promise<void>
  clearingCache: boolean
  codeCheckFailed: boolean
  compiling: boolean
  draft: boolean
  editedSinceCompileStarted: boolean
  error?: string
  fileList?: PdfFileDataList
  hasChanges: boolean
  highlights?: HighlightData[]
  isProjectOwner: boolean
  lastCompileOptions: CompileOptions
  logEntries?: LogEntries
  logEntryAnnotations?: Record<string, Annotation[]>
  pdfDownloadUrl?: string
  pdfFile?: PDFFile | null
  pdfUrl?: string
  pdfViewer: 'pdfjs' | 'native'
  position?: PdfScrollPosition
  rawLog?: string
  setAutoCompile: (value: boolean) => void
  setDraft: (value: boolean) => void
  setError: (value: string | undefined) => void
  setHighlights: (value: HighlightData[] | undefined) => void
  setPosition: Dispatch<SetStateAction<PdfScrollPosition | undefined>>
  setShowLogs: (value: boolean) => void
  toggleLogs: () => void
  setStopOnFirstError: (value: boolean) => void
  setStopOnValidationError: (value: boolean) => void
  showLogs: boolean
  stopOnFirstError: boolean
  stopOnValidationError: boolean
  stoppedOnFirstError: boolean
  uncompiled: boolean
  validationIssues?: ValidationProblems
  animateCompileDropdownArrowSet: (value: boolean) => void
  setAnimateCompileDropdownArrow: (value: boolean) => void
  recompileFromScratch: () => void
  setCompiling: (value: boolean) => void
  startCompile: (options?: CompileOptions) => Promise<void>
  stopCompile: () => void
  setChangedAt: (value: number) => void
  cleanupCompileResult: () => void
  syncToEntry: (entry: SourceLocation, keepCurrentView?: boolean) => void
  darkModePdf: boolean
  setDarkModePdf: (value: boolean) => void
  activeOverallTheme: 'light' | 'dark'
  /** Whether a compile has finished since the project was opened. */
  hasCompiled: boolean
  /** The source has changed since the PDF was made. */
  stale: boolean
  markEdited: () => void
  stop: () => void
}

const CompileContext = createContext<CompileValue | undefined>(undefined)

export function CompileProvider({ children }: { children: ReactNode }) {
  const { projectId, project, files, isOwner } = useProject()
  const rootDocId = project.rootDocId
  const editor = useEditor()
  const { pdfPreviewOpen, showLogs, setShowLogs, toggleLogs } = useLayout()
  const settings = useSettings()
  const { pdfViewer, syntaxValidation, darkModePdf, overallTheme } = settings
  const { wikiEnabled } = useSite()

  // the hints link to the help pages only on a site that has them
  useEffect(() => {
    setWikiEnabled(wikiEnabled)
  }, [wikiEnabled])

  // whether a compile is in progress
  const [compiling, setCompiling] = useState(false)

  // the log entries parsed from the compile output log
  const [logEntries, setLogEntries] = useState<LogEntries | undefined>()

  // annotations for display in the editor, built from the log entries
  const [logEntryAnnotations, setLogEntryAnnotations] = useState<undefined | Record<string, Annotation[]>>()

  // low level details for the viewer
  const [pdfFile, setPdfFile] = useState<PDFFile | null | undefined>()

  // the project is considered to be "uncompiled" if a doc has changed since the last compile started.
  const [uncompiled, setUncompiled] = useState(false)

  // whether a doc has been edited since the last compile started
  const [editedSinceCompileStarted, setEditedSinceCompileStarted] = useState(false)

  // data received in response to a compile request
  const [data, setData] = useState<CompileData>()

  // the rootDocId used in the most recent compile request, which may not be the
  // same as the project rootDocId. This is used to calculate correct paths when
  // parsing the compile logs
  const lastCompileRootDocId = data ? (data.rootDocId ?? rootDocId) : null

  // whether the project has been compiled yet
  const [compiledOnce, setCompiledOnce] = useState(false)
  const [hasCompiled, setHasCompiled] = useState(false)

  // whether the cache is being cleared
  const [clearingCache, setClearingCache] = useState(false)

  // whether the compile dropdown arrow should be animated
  const [animateCompileDropdownArrow, setAnimateCompileDropdownArrow] = useState(false)

  // an error that occurred
  const [error, setError] = useState<string>()

  // the list of files that can be downloaded
  const [fileList, setFileList] = useState<PdfFileDataList>()

  // the raw contents of the log file
  const [rawLog, setRawLog] = useState<string>()

  // validation issues from the compiler
  const [validationIssues, setValidationIssues] = useState<ValidationProblems>()

  // areas to highlight on the PDF, from synctex
  const [highlights, setHighlights] = useState<HighlightData[] | undefined>()

  const [position, setPosition] = usePdfScrollPosition(lastCompileRootDocId)

  // whether autocompile is switched on
  const [autoCompile, setAutoCompile] = usePersistedState(`autocompile_enabled:${projectId}`, false)

  // whether the compile should run in draft mode
  const [draft, setDraft] = usePersistedState(`draft:${projectId}`, false)

  // whether compiling should stop on first error
  const [stopOnFirstError, setStopOnFirstError] = usePersistedState(`stop_on_first_error:${projectId}`, false)

  // whether the last compile stopped on first error
  const [stoppedOnFirstError, setStoppedOnFirstError] = useState(false)

  // whether compiling should be prevented if there are linting errors
  const [stopOnValidationError, setStopOnValidationError] = usePersistedState(`stop_on_validation_error:${projectId}`, true)

  // whether the editor linter found errors
  const [hasLintingError, setHasLintingError] = useState(false)

  useEventListener(
    'editor:lint' as keyof WindowEventMap,
    useCallback((event: Event) => {
      setHasLintingError((event as CustomEvent<{ hasLintingError: boolean }>).detail.hasLintingError)
    }, [])
  )

  // the timestamp that a doc was last changed
  const [changedAt, setChangedAt] = useState(0)

  const { signal } = useAbortController()

  const cleanupCompileResult = useCallback(() => {
    setPdfFile(undefined)
    setLogEntries(undefined)
    setLogEntryAnnotations({})
  }, [])

  const compilingRef = useRef(false)

  useEffect(() => {
    compilingRef.current = compiling
  }, [compiling])

  const filesRef = useRef(files)
  useEffect(() => {
    filesRef.current = files
  }, [files])

  const rootDocIdRef = useRef(rootDocId)
  useEffect(() => {
    rootDocIdRef.current = rootDocId
  }, [rootDocId])

  const unsavedRef = useRef(editor.unsaved)
  useEffect(() => {
    unsavedRef.current = editor.unsaved
  }, [editor.unsaved])

  // the document compiler
  const [compiler] = useState(() => {
    return new DocumentCompiler({
      projectId,
      setChangedAt,
      setCompiling,
      setData,
      setError,
      cleanupCompileResult,
      compilingRef,
      signal,
      hasPendingChanges: () => unsavedRef.current,
      getRootDocId: () => rootDocIdRef.current,
    })
  })

  // keep draft setting in sync with the compiler
  useEffect(() => {
    compiler.setOption('draft', draft)
  }, [compiler, draft])

  // keep stop on first error setting in sync with the compiler
  useEffect(() => {
    compiler.setOption('stopOnFirstError', stopOnFirstError)
  }, [compiler, stopOnFirstError])

  useEffect(() => {
    setUncompiled(changedAt > 0)
  }, [changedAt])

  useEffect(() => {
    setEditedSinceCompileStarted(changedAt > 0)
  }, [changedAt])

  // always compile the PDF once after opening the project, after the doc has loaded
  const currentDocId = editor.current?.id
  const docLoaded = Boolean(currentDocId) && !editor.loading
  useEffect(() => {
    if (!compiledOnce && docLoaded) {
      setCompiledOnce(true)
      compiler.compile({ isAutoCompileOnLoad: true })
    }
  }, [compiledOnce, docLoaded, compiler])

  // handle the data returned from a compile request
  // note: this should _only_ run when `data` changes
  useEffect(() => {
    const abortController = new AbortController()

    if (data) {
      if (data.outputFiles) {
        const outputFiles = new Map<string, OutputFile>()

        for (const outputFile of data.outputFiles) {
          outputFiles.set(outputFile.path, { ...outputFile })
        }

        // set the PDF context
        if (data.status === 'success') {
          setPdfFile(handleOutputFiles(outputFiles))
        }

        setFileList(buildFileList(outputFiles))

        // handle log files (asynchronously)
        setLogEntryAnnotations(undefined)
        setLogEntries(undefined)
        setRawLog(undefined)

        handleLogFiles(outputFiles, data.status, Boolean(data.options.stopOnFirstError), abortController.signal).then(result => {
          if (abortController.signal.aborted) {
            return
          }
          setRawLog(result.log ?? undefined)
          setLogEntries(result.logEntries)
          setLogEntryAnnotations(
            buildLogEntryAnnotations(result.logEntries.all, filesRef.current, data.rootDocId ?? rootDocIdRef.current)
          )
        })
      }

      switch (data.status) {
        case 'success':
          setError(undefined)
          setShowLogs(false)
          break

        case 'stopped-on-first-error':
          setError(undefined)
          setShowLogs(true)
          break

        case 'clsi-maintenance':
        case 'compile-in-progress':
        case 'exited':
        case 'failure':
        case 'project-too-large':
        case 'rate-limited':
        case 'terminated':
        case 'too-recently-compiled':
          setError(data.status)
          break

        case 'timedout':
          setError('timedout')
          break

        case 'autocompile-backoff':
          if (!data.options.isAutoCompileOnLoad) {
            setError('autocompile-disabled')
            setAutoCompile(false)
          }
          break

        case 'unavailable':
          setError('clsi-unavailable')
          break

        case 'validation-problems':
          setError('validation-problems')
          setValidationIssues((data as unknown as { validationProblems?: ValidationProblems }).validationProblems)
          break

        default:
          setError('error')
          break
      }

      setStoppedOnFirstError(data.status === 'stopped-on-first-error')
      setHasCompiled(true)
    }

    return () => {
      abortController.abort()
    }
  }, [data, setAutoCompile, setShowLogs])

  // switch to logs if there's an error
  useEffect(() => {
    if (error) {
      setShowLogs(true)
    }
  }, [error, setShowLogs])

  // whether there has been an autocompile linting error, if syntax validation is switched on
  const autoCompileLintingError = Boolean(autoCompile && syntaxValidation && hasLintingError)

  const codeCheckFailed = stopOnValidationError && autoCompileLintingError

  // the project is available for auto-compiling
  // (autocompile is enabled, the PDF preview is open, and the code check (if enabled) hasn't failed)
  const canAutoCompile = Boolean(autoCompile && pdfPreviewOpen && !codeCheckFailed)

  // show that the project has pending changes
  const hasChanges = Boolean(canAutoCompile && uncompiled && compiledOnce)

  // call the debounced autocompile function if the project is available for auto-compiling and it has changed
  useEffect(() => {
    if (canAutoCompile) {
      if (changedAt > 0) {
        compiler.debouncedAutoCompile()
      }
    } else {
      compiler.debouncedAutoCompile.cancel()
    }
  }, [compiler, canAutoCompile, changedAt])

  // cancel debounced recompile on unmount
  useEffect(() => {
    return () => {
      compiler.debouncedAutoCompile.cancel()
    }
  }, [compiler])

  // start a compile manually
  const startCompile = useCallback(
    (options?: CompileOptions) => {
      setCompiledOnce(true)
      return compiler.compile(options)
    },
    [compiler]
  )

  // stop a compile manually
  const stopCompile = useCallback(() => {
    return compiler.stopCompile()
  }, [compiler])

  // clear the compile cache
  const clearCache = useCallback(() => {
    setClearingCache(true)

    return compiler
      .clearCache()
      .then(() => {
        setFileList(undefined)
        setPdfFile(undefined)
      })
      .finally(() => {
        setClearingCache(false)
      })
  }, [compiler])

  const { openDocWithId } = useOpenDocAtLine()
  const { entryByPath } = useProject()

  const syncToEntry = useCallback(
    (entry: SourceLocation, keepCurrentView = false) => {
      if (!entry.file) {
        return
      }
      const path = entry.file.replace(/^\.\//, '')
      const result = entryByPath(path) ?? entryByPath('/' + path)

      if (result && result.kind === 'doc') {
        openDocWithId(result.id, {
          gotoLine: typeof entry.line === 'number' ? entry.line : entry.line ? Number(entry.line) : undefined,
          gotoColumn: entry.column ?? undefined,
          keepCurrentView,
        })
      }
    },
    [entryByPath, openDocWithId]
  )

  // clear the cache then run a compile, triggered by a menu item
  const recompileFromScratch = useCallback(() => {
    clearCache().then(() => {
      compiler.compile({ incremental: false })
    })
  }, [clearCache, compiler])

  // the compile shortcuts, and the editor's save commands
  const startCompileNow = useCallback(() => {
    void startCompile()
  }, [startCompile])
  useCompileTriggers(startCompileNow)

  // the editor asking to see one entry of the log
  useLogEvents(setShowLogs)

  // typing marks the PDF as behind the text
  const markEdited = useCallback(() => setChangedAt(Date.now()), [])

  const setDarkModePdf = useCallback((value: boolean) => settings.set('darkModePdf', value), [settings])

  // After a compile, the compiler sets `data.options` to the options that were
  // used for that compile.
  const lastCompileOptions = useMemo(() => data?.options || {}, [data])

  const value = useMemo<CompileValue>(
    () => ({
      animateCompileDropdownArrow,
      autoCompile,
      clearCache,
      clearingCache,
      codeCheckFailed,
      compiling,
      draft,
      editedSinceCompileStarted,
      error,
      fileList,
      hasChanges,
      highlights,
      isProjectOwner: isOwner,
      lastCompileOptions,
      logEntryAnnotations,
      logEntries,
      pdfDownloadUrl: pdfFile?.pdfDownloadUrl,
      pdfFile,
      pdfUrl: pdfFile?.pdfUrl,
      pdfViewer,
      position,
      rawLog,
      recompileFromScratch,
      animateCompileDropdownArrowSet: setAnimateCompileDropdownArrow,
      setAnimateCompileDropdownArrow,
      setAutoCompile,
      setCompiling,
      setDraft,
      setError,
      setHighlights,
      setPosition,
      setShowLogs,
      toggleLogs,
      setStopOnFirstError,
      setStopOnValidationError,
      showLogs,
      startCompile,
      stopCompile,
      stopOnFirstError,
      stopOnValidationError,
      stoppedOnFirstError,
      uncompiled,
      validationIssues,
      setChangedAt,
      cleanupCompileResult,
      syncToEntry,
      darkModePdf,
      setDarkModePdf,
      activeOverallTheme: overallTheme,
      hasCompiled,
      stale: uncompiled,
      markEdited,
      stop: stopCompile,
    }),
    [
      animateCompileDropdownArrow,
      autoCompile,
      clearCache,
      clearingCache,
      codeCheckFailed,
      compiling,
      draft,
      editedSinceCompileStarted,
      error,
      fileList,
      hasChanges,
      highlights,
      isOwner,
      lastCompileOptions,
      logEntries,
      logEntryAnnotations,
      position,
      pdfFile,
      pdfViewer,
      rawLog,
      recompileFromScratch,
      setAutoCompile,
      setDraft,
      setPosition,
      setShowLogs,
      toggleLogs,
      setStopOnFirstError,
      setStopOnValidationError,
      showLogs,
      startCompile,
      stopCompile,
      stopOnFirstError,
      stopOnValidationError,
      stoppedOnFirstError,
      uncompiled,
      validationIssues,
      cleanupCompileResult,
      syncToEntry,
      darkModePdf,
      setDarkModePdf,
      overallTheme,
      hasCompiled,
      markEdited,
    ]
  )

  return <CompileContext.Provider value={value}>{children}</CompileContext.Provider>
}

export function useCompile(): CompileValue {
  const value = useContext(CompileContext)
  if (!value) {
    throw new Error('useCompile must be used inside a CompileProvider')
  }
  return value
}
