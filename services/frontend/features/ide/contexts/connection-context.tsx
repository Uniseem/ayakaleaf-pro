'use client'

/**
 * The live connection to the project.
 *
 * One socket for the whole editor, opened here so that the file tree, the
 * document and the list of who else is here all share it rather than each
 * opening their own.
 *
 * What this does not do is hide the connection's state. An editor that looks
 * the same whether or not it is connected is an editor that quietly stops
 * saving, so the state is published and the toolbar shows it.
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
import { SocketClient, type SocketState } from '@/lib/socketio'
import { useProject } from './project-context'

/** Somebody else with this project open. */
export type PresentUser = {
  clientId: string
  id: string
  name: string
  email?: string
  /** Where their cursor is, when they have told us. */
  docId?: string
  row?: number
  column?: number
}

export type ConnectionValue = {
  socket: SocketClient | null
  state: SocketState
  /** Everybody else here, not including this person. */
  others: PresentUser[]
  /** Tells everybody else where this person's cursor is. */
  reportPosition: (docId: string, row: number, column: number) => void
}

const ConnectionContext = createContext<ConnectionValue | undefined>(undefined)

export function ConnectionProvider({ children }: { children: ReactNode }) {
  const { projectId } = useProject()
  const [state, setState] = useState<SocketState>('connecting')
  const [others, setOthers] = useState<PresentUser[]>([])
  const socketRef = useRef<SocketClient | null>(null)
  const [socket, setSocket] = useState<SocketClient | null>(null)

  useEffect(() => {
    const client = new SocketClient({
      projectId,
      onState: setState,
    })
    socketRef.current = client
    setSocket(client)

    const off: Array<() => void> = []

    off.push(
      client.on('connectionAccepted', () => {
        // Ask who else is here. Anybody who arrives later announces
        // themselves, so this is only needed on the way in.
        client
          .request('clientTracking.getConnectedUsers', [])
          .then(args => {
            const list = (args[0] ?? []) as RawUser[]
            setOthers(list.map(toPresentUser).filter(user => !user.isSelf))
          })
          .catch(() => {
            // Not knowing who else is here is worth nothing to report: the
            // editor works, it just does not show faces.
          })
      })
    )

    off.push(
      client.on('clientTracking.clientUpdated', (...args) => {
        const raw = args[0] as RawUser | undefined
        if (!raw?.client_id) {
          return
        }
        const user = toPresentUser(raw)
        setOthers(previous => {
          const without = previous.filter(each => each.clientId !== user.clientId)
          return user.isSelf ? without : [...without, user]
        })
      })
    )

    off.push(
      client.on('clientTracking.clientDisconnected', (...args) => {
        const clientId = String(args[0] ?? '')
        setOthers(previous => previous.filter(each => each.clientId !== clientId))
      })
    )

    // A reconnect leaves the old list stale: those clients may have gone while
    // this one was away, and each will announce itself again if it has not.
    off.push(
      client.on('connect', () => {
        setOthers([])
      })
    )

    client.connect()

    return () => {
      for (const remove of off) {
        remove()
      }
      client.close()
      socketRef.current = null
      setSocket(null)
    }
  }, [projectId])

  const reportPosition = useCallback(
    (docId: string, row: number, column: number) => {
      socketRef.current?.emit('clientTracking.updatePosition', [
        { doc_id: docId, row, column },
      ])
    },
    []
  )

  const value = useMemo<ConnectionValue>(
    () => ({ socket, state, others, reportPosition }),
    [socket, state, others, reportPosition]
  )

  return (
    <ConnectionContext.Provider value={value}>
      {children}
    </ConnectionContext.Provider>
  )
}

export function useConnection(): ConnectionValue {
  const value = useContext(ConnectionContext)
  if (!value) {
    throw new Error('useConnection must be used inside a ConnectionProvider')
  }
  return value
}

type RawUser = {
  client_id?: string
  user_id?: string
  first_name?: string
  last_name?: string
  email?: string
  doc_id?: string
  row?: number
  column?: number
  /** The server marks the entry that is this connection. */
  self?: boolean
}

function toPresentUser(raw: RawUser): PresentUser & { isSelf: boolean } {
  const name =
    [raw.first_name, raw.last_name].filter(Boolean).join(' ') ||
    raw.email ||
    'Someone'
  return {
    clientId: String(raw.client_id ?? ''),
    id: String(raw.user_id ?? ''),
    name,
    email: raw.email,
    docId: raw.doc_id,
    row: raw.row,
    column: raw.column,
    isSelf: Boolean(raw.self),
  }
}
