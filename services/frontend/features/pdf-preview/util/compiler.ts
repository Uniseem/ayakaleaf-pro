import type { Dispatch, SetStateAction } from 'react'
import { compileProject, stopCompile as stopCompileRequest, type CompileResult } from '@/lib/editor'
import { api, ApiError } from '@/lib/api'
import { debounce, type Debounced } from '@/lib/timing'
import { debugConsole } from '@/lib/debug'
import type { CompileOptions } from './types'

const AUTO_COMPILE_MAX_WAIT = 5000
// A 2 second debounce on sending user changes to the server when they aren't
// collaborating with anyone. This needs to be higher than the single-user
// flush delay, and allow for client to server latency, otherwise the compile
// runs before the change reaches the server and then again on ack.
const AUTO_COMPILE_DEBOUNCE = 2500

// If there is a pending op, wait for it to be saved before compiling
const PENDING_OP_MAX_WAIT = 10000

/** What a compile answered, plus what it was asked. */
export type CompileData = CompileResult & {
  rootDocId?: string | null
  options: CompileOptions
}

export default class DocumentCompiler {
  compilingRef: React.RefObject<boolean>
  projectId: string
  setChangedAt: Dispatch<SetStateAction<number>>
  setCompiling: Dispatch<SetStateAction<boolean>>
  setData: Dispatch<SetStateAction<CompileData | undefined>>
  setError: Dispatch<SetStateAction<string | undefined>>
  cleanupCompileResult: () => void
  signal: AbortSignal
  /** Whether anything typed is still on its way to the server. */
  hasPendingChanges: () => boolean
  getRootDocId: () => string | undefined
  error: Error | undefined
  timer: number
  defaultOptions: CompileOptions
  debouncedAutoCompile: Debounced<() => void>

  constructor({
    compilingRef,
    projectId,
    setChangedAt,
    setCompiling,
    setData,
    setError,
    cleanupCompileResult,
    signal,
    hasPendingChanges,
    getRootDocId,
  }: {
    compilingRef: React.RefObject<boolean>
    projectId: string
    setChangedAt: Dispatch<SetStateAction<number>>
    setCompiling: Dispatch<SetStateAction<boolean>>
    setData: Dispatch<SetStateAction<CompileData | undefined>>
    setError: Dispatch<SetStateAction<string | undefined>>
    cleanupCompileResult: () => void
    signal: AbortSignal
    hasPendingChanges: () => boolean
    getRootDocId: () => string | undefined
  }) {
    this.compilingRef = compilingRef
    this.projectId = projectId
    this.setChangedAt = setChangedAt
    this.setCompiling = setCompiling
    this.setData = setData
    this.setError = setError
    this.cleanupCompileResult = cleanupCompileResult
    this.signal = signal
    this.hasPendingChanges = hasPendingChanges
    this.getRootDocId = getRootDocId

    this.error = undefined
    this.timer = 0
    this.defaultOptions = {
      draft: false,
      stopOnFirstError: false,
    }

    this.debouncedAutoCompile = debounce(() => {
      this.compile({ isAutoCompileOnChange: true })
    }, AUTO_COMPILE_DEBOUNCE)
  }

  /** Waits for what was typed to reach the server, up to a limit. */
  private async awaitPendingChanges() {
    const started = Date.now()
    while (this.hasPendingChanges() && Date.now() - started < PENDING_OP_MAX_WAIT) {
      if (this.signal.aborted) {
        return
      }
      await new Promise(resolve => window.setTimeout(resolve, 100))
    }
  }

  // The main "compile" function.
  // Call this directly to run a compile now, otherwise call debouncedAutoCompile.
  async compile(options: CompileOptions = {}) {
    options = { ...this.defaultOptions, ...options }

    // set "compiling" to true (in the React component's state), and return if it was already true
    const wasCompiling = this.compilingRef.current
    this.setCompiling(true)

    if (wasCompiling) {
      if (options.isAutoCompileOnChange) {
        this.debouncedAutoCompile()
      }
      return
    }

    try {
      await this.awaitPendingChanges()

      // reset values
      this.setChangedAt(0)

      const rootDocId = this.getRootDocId()

      const data: CompileData = {
        ...(await compileProject(
          this.projectId,
          {
            rootDocId,
            draft: options.draft,
            // use incremental compile normally, but revert to a full compile
            // if there was previously a server error
            incremental: !this.error && options.incremental !== false,
            stopOnFirstError: options.stopOnFirstError,
          },
          this.signal
        )),
        options: { ...options },
        rootDocId,
      }

      // unset the error before it's set again later, so that components are recreated
      this.setError(undefined)
      this.error = undefined

      this.setData(data)
    } catch (error) {
      debugConsole.error(error)
      this.error = error as Error
      this.cleanupCompileResult()
      this.setError(error instanceof ApiError && error.status === 429 ? 'rate-limited' : 'error')
    } finally {
      this.setCompiling(false)
    }
  }

  // send a request to stop the current compile
  stopCompile() {
    // NOTE: no stoppingCompile state, as this should happen fairly quickly
    // and doesn't matter if it runs twice.
    return stopCompileRequest(this.projectId)
      .catch(error => {
        debugConsole.error(error)
        this.setError('error')
      })
      .finally(() => {
        this.setCompiling(false)
      })
  }

  // send a request to clear the cache
  clearCache() {
    return api<void>(`/api/projects/${this.projectId}/output`, { method: 'DELETE', signal: this.signal }).catch(error => {
      // a server without the endpoint has nothing to clear; the next compile
      // runs from scratch anyway
      if (error instanceof ApiError && (error.status === 404 || error.status === 405)) {
        return
      }
      debugConsole.error(error)
      this.setError('clear-cache')
    })
  }

  setOption<Key extends keyof CompileOptions>(option: Key, value: CompileOptions[Key]) {
    this.defaultOptions[option] = value
  }
}
