'use client'

/**
 * A socket.io 0.9 client.
 *
 * The real-time service speaks socket.io 0.9, which predates the modern
 * protocol by about a decade and which nothing current implements. The
 * alternative to this file is loading Overleaf's forked client as a global
 * script -- jQuery-era code, untyped, outside the bundle, and a second copy of
 * a reconnect policy.
 *
 * The protocol is small enough to be worth writing instead:
 *
 *   1. GET /socket.io/1/?t=<now>&projectId=<id>   with the session cookie
 *      answers `sid:heartbeat:close:transports`
 *   2. open a websocket to /socket.io/1/websocket/<sid>
 *   3. frames are `type:id[+]:endpoint:data`
 *   4. an event is type 5 with data {"name": string, "args": unknown[]}
 *   5. an ack is type 6 with data `<id>+<json args>`
 *
 * The query flags go on the handshake, not on the upgrade: the server reads
 * them off that request and keeps them for the life of the connection.
 */

/** Packet types, from the protocol's own parser. */
const DISCONNECT = 0
const CONNECT = 1
const HEARTBEAT = 2
const EVENT = 5
const ACK = 6
const ERROR = 7

export type SocketState =
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'disconnected'
  | 'failed'

type Listener = (...args: unknown[]) => void

/** How long to wait before a reconnect, growing to a ceiling. */
const RECONNECT_BASE = 1000
const RECONNECT_CEILING = 20000
const MAX_ATTEMPTS = 12

export type SocketOptions = {
  /** Sent on the handshake, which is where this server expects it. */
  projectId: string
  /** Told about every state change, for the banner the editor shows. */
  onState?: (state: SocketState) => void
}

export class SocketClient {
  private ws: WebSocket | null = null
  private readonly listeners = new Map<string, Set<Listener>>()
  /** Callbacks waiting for an ack, by the id sent with the request. */
  private readonly pending = new Map<
    string,
    { resolve: (args: unknown[]) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout> }
  >()
  private nextId = 1
  private heartbeat: ReturnType<typeof setInterval> | null = null
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private attempts = 0
  private closed = false
  private state: SocketState = 'connecting'

  constructor(private readonly options: SocketOptions) {}

  /** Opens the connection, and keeps it open. */
  connect() {
    this.closed = false
    void this.open()
  }

  /** Closes for good. A client closed this way does not reconnect. */
  close() {
    this.closed = true
    this.clearTimers()
    this.setState('disconnected')
    if (this.ws) {
      // Remove handlers first, or onclose schedules a reconnect on the way out.
      this.ws.onclose = null
      this.ws.onerror = null
      this.ws.onmessage = null
      this.ws.close()
      this.ws = null
    }
    for (const [, waiting] of this.pending) {
      clearTimeout(waiting.timer)
      waiting.reject(new Error('The connection was closed.'))
    }
    this.pending.clear()
  }

  on(event: string, listener: Listener): () => void {
    const set = this.listeners.get(event) ?? new Set()
    set.add(listener)
    this.listeners.set(event, set)
    return () => set.delete(listener)
  }

  /**
   * Sends an event and waits for the server's ack.
   *
   * Everything the editor asks for is one of these: joining a document,
   * sending an edit. A plain emit with no reply is `emit`.
   */
  request(name: string, args: unknown[] = [], timeout = 30000): Promise<unknown[]> {
    return new Promise((resolve, reject) => {
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
        reject(new Error('Not connected.'))
        return
      }
      const id = String(this.nextId++)
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new Error(`${name} did not answer.`))
      }, timeout)
      this.pending.set(id, { resolve, reject, timer })
      // The trailing "+" asks for an explicit ack rather than an automatic one.
      this.send(`${EVENT}:${id}+::${JSON.stringify({ name, args })}`)
    })
  }

  /** Sends an event and does not wait. */
  emit(name: string, args: unknown[] = []) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.send(`${EVENT}:::${JSON.stringify({ name, args })}`)
    }
  }

  get connected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  private setState(state: SocketState) {
    if (this.state !== state) {
      this.state = state
      this.options.onState?.(state)
    }
  }

  private send(frame: string) {
    this.ws?.send(frame)
  }

  private async open() {
    if (this.closed) {
      return
    }
    this.setState(this.attempts === 0 ? 'connecting' : 'reconnecting')

    let sid: string
    let heartbeatSeconds: number
    try {
      const answer = await fetch(
        `/socket.io/1/?t=${Date.now()}&projectId=${encodeURIComponent(this.options.projectId)}`,
        { credentials: 'include', cache: 'no-store' }
      )
      if (!answer.ok) {
        throw new Error(`handshake answered ${answer.status}`)
      }
      const [id, heartbeat] = (await answer.text()).split(':')
      if (!id) {
        throw new Error('handshake gave no session')
      }
      sid = id
      heartbeatSeconds = Number(heartbeat) || 25
    } catch {
      this.retry()
      return
    }
    if (this.closed) {
      return
    }

    const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(
      `${scheme}://${window.location.host}/socket.io/1/websocket/${sid}`
    )
    this.ws = ws

    ws.onmessage = event => this.receive(String(event.data))

    ws.onclose = () => {
      this.clearHeartbeat()
      if (!this.closed) {
        this.setState('reconnecting')
        this.retry()
      }
    }

    ws.onerror = () => {
      // onclose always follows, and doing the work in one place keeps the
      // reconnect from being scheduled twice.
      ws.close()
    }

    // The server sends its own heartbeats and expects a reply; this is the
    // safety net for a connection that goes quiet without closing, which is
    // what a dropped mobile connection looks like.
    this.clearHeartbeat()
    this.heartbeat = setInterval(
      () => {
        if (ws.readyState === WebSocket.OPEN) {
          this.send(`${HEARTBEAT}::`)
        }
      },
      Math.max(5, heartbeatSeconds - 5) * 1000
    )
  }

  private receive(frame: string) {
    const match = /^([^:]+):([0-9]+)?(\+)?:([^:]*)?:?([\s\S]*)?$/.exec(frame)
    if (!match) {
      return
    }
    const type = Number(match[1])
    const data = match[5] ?? ''

    switch (type) {
      case CONNECT:
        this.attempts = 0
        this.setState('connected')
        this.dispatch('connect', [])
        break

      case HEARTBEAT:
        this.send(`${HEARTBEAT}::`)
        break

      case EVENT: {
        try {
          const parsed = JSON.parse(data) as { name: string; args?: unknown[] }
          this.dispatch(parsed.name, parsed.args ?? [])
        } catch {
          // A frame this client cannot read is not a reason to drop the
          // connection: the next one is probably fine.
        }
        break
      }

      case ACK: {
        // `<id>+<json args>`, or just `<id>` when there were none.
        const plus = data.indexOf('+')
        const id = plus === -1 ? data : data.slice(0, plus)
        const waiting = this.pending.get(id)
        if (!waiting) {
          break
        }
        this.pending.delete(id)
        clearTimeout(waiting.timer)
        let args: unknown[] = []
        if (plus !== -1) {
          try {
            args = JSON.parse(data.slice(plus + 1)) as unknown[]
          } catch {
            args = []
          }
        }
        // By convention the first argument is an error or null, so a caller
        // gets a rejected promise rather than having to check every time.
        const [failure] = args
        if (failure) {
          waiting.reject(new Error(errorMessage(failure)))
        } else {
          waiting.resolve(args.slice(1))
        }
        break
      }

      case ERROR:
        this.dispatch('error', [data])
        break

      case DISCONNECT:
        this.close()
        this.dispatch('disconnect', [])
        break
    }
  }

  private dispatch(event: string, args: unknown[]) {
    const set = this.listeners.get(event)
    if (!set) {
      return
    }
    for (const listener of [...set]) {
      try {
        listener(...args)
      } catch {
        // One listener throwing must not stop the others, and must not take
        // the connection with it.
      }
    }
  }

  private retry() {
    if (this.closed || this.reconnectTimer) {
      return
    }
    this.attempts++
    if (this.attempts > MAX_ATTEMPTS) {
      this.setState('failed')
      return
    }
    // Exponential, with jitter, so that a service coming back up is not hit by
    // every editor that was connected to it at the same instant.
    const wait = Math.min(
      RECONNECT_CEILING,
      RECONNECT_BASE * 2 ** (this.attempts - 1)
    )
    const jittered = wait * (0.5 + Math.random() * 0.5)
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      void this.open()
    }, jittered)
  }

  private clearHeartbeat() {
    if (this.heartbeat) {
      clearInterval(this.heartbeat)
      this.heartbeat = null
    }
  }

  private clearTimers() {
    this.clearHeartbeat()
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }
}

function errorMessage(failure: unknown): string {
  if (typeof failure === 'string') {
    return failure
  }
  if (failure && typeof failure === 'object') {
    const held = failure as { message?: unknown }
    if (typeof held.message === 'string') {
      return held.message
    }
  }
  return 'That did not work.'
}
