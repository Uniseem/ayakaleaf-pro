'use client'

/**
 * The documents the editor has open.
 *
 * One document is in front, but the others stay joined: switching files and
 * coming back should not lose the cursor, and must not drop an edit that has
 * not reached the server yet.
 *
 * Writes go over the socket, not over HTTP. That is not a preference -- it is
 * the only way two people can edit one file. An HTTP save sends the whole
 * document and the last one to arrive wins, which silently deletes whatever
 * the other person wrote in between. The socket sends operations, which the
 * server transforms against each other so that both edits survive. The
 * consequence is that a disconnected editor cannot save, and says so, rather
 * than pretending and losing somebody's work at reconnect.
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
import type { FileEntry } from '@/lib/editor'
import { messageFor } from '@/lib/api'
import { usePersistedState } from '@/lib/hooks'
import { DocSession } from '@/features/source-editor/doc-session'
import { useProject } from './project-context'
import { useConnection } from './connection-context'

/** A document as this session holds it. */
export type OpenDoc = {
  id: string
  name: string
  path: string
  content: string
  cursor: number
  scrollTop: number
}

export type EditorValue = {
  current: OpenDoc | null
  currentFile: FileEntry | null
  loading: boolean
  error: string | null
  /** Set when the text was replaced under the person by a resync. */
  notice: string | null
  dismissNotice: () => void

  open: (entry: FileEntry) => void
  close: () => void
  change: (content: string) => void
  rememberPosition: (cursor: number, scrollTop: number) => void

  /** Whether anything is waiting to reach the server. */
  unsaved: boolean
  /** Whether edits can be made at all right now. */
  editable: boolean
  /** A revision that changes when the text is replaced from outside, so the
   * editor knows to take the new text rather than treat it as its own. */
  revision: number
}

const EditorContext = createContext<EditorValue | undefined>(undefined)

export function EditorProvider({ children }: { children: ReactNode }) {
  const { projectId, project, files, entryById, canWrite } = useProject()
  const { socket, state } = useConnection()

  const [currentId, setCurrentId] = usePersistedState<string | null>(
    `ide.openDoc.${projectId}`,
    null
  )
  const [currentFile, setCurrentFile] = useState<FileEntry | null>(null)
  const [text, setText] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [unsaved, setUnsaved] = useState(false)
  const [revision, setRevision] = useState(0)

  // Where the cursor was in each document, so switching back restores it.
  const positions = useRef<Map<string, { cursor: number; scrollTop: number }>>(
    new Map()
  )
  const session = useRef<DocSession | null>(null)
  // The document the live session is for, to tell a switch from a re-render.
  const joined = useRef<string | null>(null)

  const connected = state === 'connected'

  // Join the open document, and leave it when it changes or the socket goes.
  useEffect(() => {
    if (!socket || !connected || !currentId) {
      return
    }
    let live = true
    const docSession = new DocSession(socket, currentId, {
      onRemote: next => {
        if (live) {
          setText(next)
          setRevision(value => value + 1)
        }
      },
      onResync: next => {
        if (live) {
          setText(next)
          setRevision(value => value + 1)
          setNotice(
            'This document was reloaded from the server. An edit could not be applied.'
          )
        }
      },
      onError: message => {
        if (live) {
          setError(message)
        }
      },
    })

    setLoading(true)
    setError(null)
    docSession
      .join()
      .then(content => {
        if (!live) {
          docSession.leave()
          return
        }
        session.current = docSession
        joined.current = currentId
        setText(content)
        setRevision(value => value + 1)
        setLoading(false)
      })
      .catch((thrown: unknown) => {
        if (live) {
          setError(messageFor(thrown))
          setLoading(false)
        }
      })

    return () => {
      live = false
      docSession.leave()
      if (session.current === docSession) {
        session.current = null
        joined.current = null
      }
    }
  }, [socket, connected, currentId])

  // Send whatever was waiting when the connection comes back.
  useEffect(() => {
    if (connected) {
      session.current?.resume()
    }
  }, [connected])

  // Keep the "unsaved" flag honest without polling the session on render.
  useEffect(() => {
    const timer = setInterval(() => {
      setUnsaved(session.current?.hasUnsent ?? false)
    }, 500)
    return () => clearInterval(timer)
  }, [])

  const open = useCallback(
    (entry: FileEntry) => {
      if (entry.kind === 'folder') {
        return
      }
      if (entry.kind === 'file') {
        setCurrentFile(entry)
        return
      }
      setCurrentFile(null)
      setCurrentId(entry.id)
    },
    [setCurrentId]
  )

  const close = useCallback(() => setCurrentFile(null), [])

  const change = useCallback(
    (content: string) => {
      if (!canWrite) {
        return
      }
      session.current?.localChange(content)
    },
    [canWrite]
  )

  const rememberPosition = useCallback(
    (cursor: number, scrollTop: number) => {
      if (currentId) {
        positions.current.set(currentId, { cursor, scrollTop })
      }
    },
    [currentId]
  )

  // Open the root document when nothing else is, so a project opens on
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

  // A document that was open and has since been deleted.
  useEffect(() => {
    if (currentId && files.length > 0 && !entryById(currentId)) {
      setCurrentId(null)
      setText('')
    }
  }, [currentId, files, entryById, setCurrentId])

  // Warn before leaving with something unsent. It cannot be awaited, so this
  // is a warning rather than a save.
  useEffect(() => {
    const handler = (event: BeforeUnloadEvent) => {
      if (session.current?.hasUnsent) {
        event.preventDefault()
        event.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [])

  const value = useMemo<EditorValue>(() => {
    const entry = currentId ? entryById(currentId) : undefined
    const current: OpenDoc | null =
      currentId && entry
        ? {
            id: currentId,
            name: entry.name,
            path: entry.path,
            content: text,
            cursor: positions.current.get(currentId)?.cursor ?? 0,
            scrollTop: positions.current.get(currentId)?.scrollTop ?? 0,
          }
        : null

    return {
      current,
      currentFile,
      loading,
      error,
      notice,
      dismissNotice: () => setNotice(null),
      open,
      close,
      change,
      rememberPosition,
      unsaved,
      // Read-only access cannot edit, and neither can anybody whose
      // connection is down: there is nowhere for the edit to go.
      editable: canWrite && connected && joined.current === currentId,
      revision,
    }
  }, [
    currentId,
    entryById,
    text,
    currentFile,
    loading,
    error,
    notice,
    open,
    close,
    change,
    rememberPosition,
    unsaved,
    canWrite,
    connected,
    revision,
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
