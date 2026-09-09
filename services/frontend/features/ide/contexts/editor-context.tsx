'use client'

/**
 * The documents the editor has open.
 *
 * One document is in front, but the others stay loaded: switching files and
 * coming back should not lose the cursor, the scroll position, or -- much
 * worse -- an edit that had not been saved yet. So documents are kept in a map
 * and only dropped when the project is left.
 *
 * Saving is debounced rather than immediate. Every keystroke is not a version
 * worth keeping, and the compile that follows an edit needs the server to have
 * the text, not to have written it forty times on the way there.
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
import { getDocument, saveDocument, type FileEntry } from '@/lib/editor'
import { messageFor } from '@/lib/api'
import { usePersistedState } from '@/lib/hooks'
import { useProject } from './project-context'

/** How long after the last keystroke the document is written. */
const SAVE_DELAY = 1200

/** A document as this session holds it. */
export type OpenDoc = {
  id: string
  name: string
  path: string
  content: string
  /** The version the server had when this was fetched. */
  version: number
  /** Whether there are edits the server has not been told about. */
  dirty: boolean
  /** Where the cursor was when this document was last in front. */
  cursor: number
  scrollTop: number
}

export type EditorValue = {
  /** The document in front, or null when nothing is open. */
  current: OpenDoc | null
  /** A binary file being viewed instead of a document. */
  currentFile: FileEntry | null
  loading: boolean
  error: string | null

  open: (entry: FileEntry) => void
  close: () => void
  /** Records an edit, without writing it yet. */
  change: (content: string) => void
  /** Writes now, rather than waiting for the debounce. */
  flush: () => Promise<void>
  rememberPosition: (cursor: number, scrollTop: number) => void

  /** Documents with edits that have not reached the server. */
  unsaved: string[]
  saving: boolean
}

const EditorContext = createContext<EditorValue | undefined>(undefined)

export function EditorProvider({ children }: { children: ReactNode }) {
  const { projectId, project, files, entryById, canWrite } = useProject()

  const [docs, setDocs] = useState<Map<string, OpenDoc>>(new Map())
  const [currentId, setCurrentId] = usePersistedState<string | null>(
    `ide.openDoc.${projectId}`,
    null
  )
  const [currentFile, setCurrentFile] = useState<FileEntry | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  // Pending writes, by document. A save that is already scheduled is replaced
  // rather than added to, so a burst of typing is one write.
  const timers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())
  const latest = useRef<Map<string, string>>(new Map())

  const write = useCallback(
    async (docId: string) => {
      const content = latest.current.get(docId)
      if (content === undefined) {
        return
      }
      latest.current.delete(docId)
      setSaving(true)
      try {
        await saveDocument(projectId, docId, content)
        setDocs(previous => {
          const held = previous.get(docId)
          // Only clean if nothing was typed while the write was in flight.
          if (!held || latest.current.has(docId)) {
            return previous
          }
          const next = new Map(previous)
          next.set(docId, { ...held, dirty: false })
          return next
        })
      } catch (thrown) {
        setError(messageFor(thrown))
        // Put it back, so the next attempt still has it and a failed save
        // does not silently lose what was typed.
        latest.current.set(docId, content)
      } finally {
        setSaving(false)
      }
    },
    [projectId]
  )

  const load = useCallback(
    async (entry: FileEntry) => {
      setLoading(true)
      setError(null)
      try {
        const fetched = await getDocument(projectId, entry.id)
        setDocs(previous => {
          const next = new Map(previous)
          next.set(entry.id, {
            id: entry.id,
            name: entry.name,
            path: entry.path,
            content: fetched.lines.join('\n'),
            version: fetched.version,
            dirty: false,
            cursor: 0,
            scrollTop: 0,
          })
          return next
        })
      } catch (thrown) {
        setError(messageFor(thrown))
      } finally {
        setLoading(false)
      }
    },
    [projectId]
  )

  const open = useCallback(
    (entry: FileEntry) => {
      if (entry.kind === 'folder') {
        return
      }
      if (entry.kind === 'file') {
        // Not text. The file view shows it instead of the editor.
        setCurrentFile(entry)
        return
      }
      setCurrentFile(null)
      setCurrentId(entry.id)
      setDocs(previous => {
        if (!previous.has(entry.id)) {
          void load(entry)
        }
        return previous
      })
    },
    [load, setCurrentId]
  )

  const close = useCallback(() => {
    setCurrentFile(null)
  }, [])

  const change = useCallback(
    (content: string) => {
      if (!currentId || !canWrite) {
        return
      }
      const docId = currentId
      setDocs(previous => {
        const held = previous.get(docId)
        if (!held || held.content === content) {
          return previous
        }
        const next = new Map(previous)
        next.set(docId, { ...held, content, dirty: true })
        return next
      })
      latest.current.set(docId, content)

      const running = timers.current.get(docId)
      if (running) {
        clearTimeout(running)
      }
      timers.current.set(
        docId,
        setTimeout(() => {
          timers.current.delete(docId)
          void write(docId)
        }, SAVE_DELAY)
      )
    },
    [currentId, canWrite, write]
  )

  const flush = useCallback(async () => {
    const pending = [...timers.current.keys()]
    for (const docId of pending) {
      const timer = timers.current.get(docId)
      if (timer) {
        clearTimeout(timer)
        timers.current.delete(docId)
      }
    }
    await Promise.all(pending.map(write))
  }, [write])

  const rememberPosition = useCallback(
    (cursor: number, scrollTop: number) => {
      if (!currentId) {
        return
      }
      setDocs(previous => {
        const held = previous.get(currentId)
        if (!held) {
          return previous
        }
        const next = new Map(previous)
        next.set(currentId, { ...held, cursor, scrollTop })
        return next
      })
    },
    [currentId]
  )

  // Open the root document when nothing else is open, so a project opens on
  // something rather than on an empty pane.
  useEffect(() => {
    if (currentId || currentFile || files.length === 0) {
      return
    }
    const root = project.rootDocId ? entryById(project.rootDocId) : undefined
    const first = root ?? files.find(entry => entry.kind === 'doc')
    if (first) {
      open(first)
    }
  }, [currentId, currentFile, files, project.rootDocId, entryById, open])

  // A document that was open and has since been deleted or renamed away.
  useEffect(() => {
    if (currentId && !entryById(currentId)) {
      setCurrentId(null)
    }
  }, [currentId, entryById, setCurrentId])

  // Write anything outstanding when the tab is closed. `beforeunload` is the
  // only chance, and it cannot await, so this is best-effort by design.
  useEffect(() => {
    const handler = (event: BeforeUnloadEvent) => {
      if (timers.current.size > 0 || latest.current.size > 0) {
        void flush()
        event.preventDefault()
        event.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [flush])

  // Clear timers on unmount so a write does not fire into a dead component.
  useEffect(() => {
    const held = timers.current
    return () => {
      held.forEach(clearTimeout)
      held.clear()
    }
  }, [])

  const value = useMemo<EditorValue>(() => {
    const current = currentId ? (docs.get(currentId) ?? null) : null
    return {
      current,
      currentFile,
      loading,
      error,
      open,
      close,
      change,
      flush,
      rememberPosition,
      unsaved: [...docs.values()].filter(doc => doc.dirty).map(doc => doc.path),
      saving,
    }
  }, [
    currentId,
    docs,
    currentFile,
    loading,
    error,
    open,
    close,
    change,
    flush,
    rememberPosition,
    saving,
  ])

  return (
    <EditorContext.Provider value={value}>{children}</EditorContext.Provider>
  )
}

export function useEditor(): EditorValue {
  const value = useContext(EditorContext)
  if (!value) {
    throw new Error('useEditor must be used inside an EditorProvider')
  }
  return value
}
