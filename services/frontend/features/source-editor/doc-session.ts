'use client'

/**
 * One document, kept in step with everybody else editing it.
 *
 * The state is the standard three: the version the server has agreed, the
 * operation sent and not yet acknowledged, and what has been typed since. Only
 * one operation is ever in flight -- the server transforms against a known
 * version, and two outstanding operations would each be written against a
 * version the other had not seen.
 *
 * The invariant everything here protects: this object's `text` is exactly what
 * the server would have if it applied inflight and then pending. If that stops
 * being true the two documents diverge silently, which is the one failure a
 * reload does not fix, so a mismatch resyncs rather than carrying on.
 */

import type { SocketClient } from '@/lib/socketio'
import { newTrackingSeed } from '@/lib/ranges'
import { apply, compose, diffToOp, transform, transformPosition, type Op } from './ot'

export type DocSessionEvents = {
  /** The text changed because somebody else edited it. */
  onRemote: (text: string, op: Op) => void
  /** The session lost track and reloaded the document from the server. */
  onResync: (text: string) => void
  onError: (message: string) => void
}

export class DocSession {
  /** What the server has agreed to, as a version number. */
  private version = 0
  /** Sent, waiting for the acknowledgement. */
  private inflight: Op | null = null
  /** Typed since, not yet sent. */
  private pending: Op = []
  /** The text as this session believes the server would have it. */
  private text = ''
  private joined = false
  private off: Array<() => void> = []
  /** Set while a remote operation is being written into the editor, so the
   * change it causes is not read back as something somebody typed. */
  private applying = false
  /**
   * Whether edits are suggestions. Setting it is what "suggesting" means: the
   * same operation is sent, and the server records it instead of applying it.
   */
  private tracking = false

  constructor(
    private readonly socket: SocketClient,
    private readonly docId: string,
    private readonly events: DocSessionEvents
  ) {}

  /** Joins the document and answers with its current text. */
  async join(): Promise<string> {
    const answer = await this.socket.request('joinDoc', [
      this.docId,
      { encodeRanges: true },
    ])
    const [lines, version] = answer as [string[] | string, number]
    this.text = Array.isArray(lines) ? lines.join('\n') : String(lines ?? '')
    this.version = Number(version) || 0
    this.inflight = null
    this.pending = []
    this.joined = true
    this.listen()
    return this.text
  }

  leave() {
    for (const remove of this.off) {
      remove()
    }
    this.off = []
    if (this.joined) {
      this.socket.emit('leaveDoc', [this.docId])
      this.joined = false
    }
  }

  get currentText(): string {
    return this.text
  }

  get atVersion(): number {
    return this.version
  }

  /** Turns suggesting on or off for this document. */
  setTracking(on: boolean) {
    this.tracking = on
  }

  get suggesting(): boolean {
    return this.tracking
  }

  /** Whether anything is waiting to reach the server. */
  get hasUnsent(): boolean {
    return this.inflight !== null || this.pending.length > 0
  }

  /**
   * Records what somebody typed.
   *
   * Takes the whole new text rather than a change: CodeMirror's changes are
   * already applied to its document by the time this is called, and deriving
   * the operation from before and after is one place to be right instead of
   * two.
   */
  localChange(next: string) {
    if (this.applying || next === this.localText()) {
      return
    }
    const op = diffToOp(this.localText(), next)
    if (op.length === 0) {
      return
    }
    this.pending = compose(this.pending, op)
    this.flush()
  }

  /** The text including everything not yet acknowledged. */
  private localText(): string {
    let result = this.text
    if (this.inflight) {
      result = apply(result, this.inflight)
    }
    if (this.pending.length > 0) {
      result = apply(result, this.pending)
    }
    return result
  }

  private listen() {
    this.off.push(
      this.socket.on('otUpdateApplied', (...args) => {
        const update = args[0] as
          | { doc?: string; op?: Op; v?: number; meta?: { source?: string } }
          | undefined
        if (!update || update.doc !== this.docId) {
          return
        }
        this.receive(update.op ?? [], Number(update.v))
      })
    )
    this.off.push(
      this.socket.on('otUpdateError', (...args) => {
        const message = typeof args[0] === 'string' ? args[0] : 'The document fell out of step.'
        this.events.onError(message)
        void this.resync()
      })
    )
  }

  /**
   * Takes in somebody else's operation.
   *
   * It arrives written against the version the server had, so it has to be
   * transformed past whatever this client has outstanding -- and those, in
   * turn, past it.
   */
  private receive(op: Op, version: number) {
    if (Number.isFinite(version)) {
      this.version = version
    }

    let incoming = op
    if (this.inflight) {
      const before = this.inflight
      // The inflight operation was sent first, so it is 'left': it wins ties.
      this.inflight = transform(before, incoming, 'left')
      incoming = transform(incoming, before, 'right')
    }
    if (this.pending.length > 0) {
      const before = this.pending
      this.pending = transform(before, incoming, 'left')
      incoming = transform(incoming, before, 'right')
    }

    try {
      this.text = apply(this.text, op)
    } catch {
      // The operation did not fit the text this session holds, which means the
      // two have already diverged. Nothing local is worth keeping at that
      // point, because it was written against text that never existed.
      void this.resync()
      return
    }

    if (incoming.length > 0) {
      this.applying = true
      try {
        this.events.onRemote(this.localText(), incoming)
      } finally {
        this.applying = false
      }
    }
  }

  /** Sends what is waiting, if nothing is already in flight. */
  private flush() {
    if (this.inflight !== null || this.pending.length === 0 || !this.socket.connected) {
      return
    }
    this.inflight = this.pending
    this.pending = []

    const sending = this.inflight
    const atVersion = this.version

    // `meta.tc` is what makes this a suggestion rather than an edit: the
    // server records the operation as a tracked change instead of applying it
    // to the text. The operation itself is identical either way.
    const update: Record<string, unknown> = {
      doc: this.docId,
      op: sending,
      v: atVersion,
    }
    if (this.tracking) {
      // A fresh seed for every operation. The server stamps each tracked
      // change with the seed plus a counter that restarts at one for every
      // update it processes -- so a seed reused across operations gives every
      // change the same id, and accepting one accepts all of them. Found
      // exactly that way: two suggestions, accept the first, both vanish.
      update.meta = { tc: newTrackingSeed() }
    }

    this.socket
      .request('applyOtUpdate', [this.docId, update])
      .then(() => {
        // Accepted. It is now part of what the server has, so it folds into
        // the agreed text and the version moves on.
        try {
          this.text = apply(this.text, sending)
        } catch {
          void this.resync()
          return
        }
        this.version = atVersion + 1
        this.inflight = null
        this.flush()
      })
      .catch((error: unknown) => {
        this.inflight = null
        this.events.onError(
          error instanceof Error ? error.message : 'That edit was not accepted.'
        )
        void this.resync()
      })
  }

  /**
   * Throws away local state and takes the server's copy.
   *
   * The last resort, and deliberately blunt: once the two disagree there is no
   * way to work out what the person meant, and guessing would put text they
   * never typed into their document.
   */
  private async resync() {
    if (!this.joined) {
      return
    }
    try {
      this.socket.emit('leaveDoc', [this.docId])
      const text = await this.join()
      this.events.onResync(text)
    } catch {
      this.events.onError('This document could not be reloaded. Refresh the page.')
    }
  }

  /** Where a cursor should be after everything outstanding is applied. */
  positionAfter(position: number): number {
    let moved = position
    if (this.inflight) {
      moved = transformPosition(moved, this.inflight)
    }
    if (this.pending.length > 0) {
      moved = transformPosition(moved, this.pending)
    }
    return moved
  }

  /**
   * Reloads the document from the server.
   *
   * Needed after anything changes it from outside this session -- accepting a
   * tracked change is done over HTTP, under document-updater's own lock, and
   * this session would otherwise keep editing against the version it had
   * before and have every operation refused.
   */
  async reload(): Promise<string> {
    this.socket.emit('leaveDoc', [this.docId])
    const text = await this.join()
    this.events.onResync(text)
    return text
  }

  /** Called when the connection comes back, to send whatever was waiting. */
  resume() {
    // Anything that was in flight when the connection dropped may or may not
    // have been applied, and there is no way to tell from here. Folding it
    // back into pending would risk applying it twice; a resync is the only
    // answer that cannot corrupt the document.
    if (this.inflight) {
      this.inflight = null
      void this.resync()
      return
    }
    this.flush()
  }
}
