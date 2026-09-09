import { ensureSyntaxTree } from '@codemirror/language'
import type { EditorState, Transaction } from '@codemirror/state'
import { IterMode, type SyntaxNodeRef } from '@lezer/common'

const TWENTY_MS = 20
const FIVE_HUNDRED_MS = 500

/** A single item in the projection. */
export abstract class ProjectionItem {
  readonly from: number = 0
  readonly to: number = 0
  readonly line: number = 0
  readonly toLine: number = 0
}

export enum ProjectionStatus {
  Pending,
  Partial,
  Complete,
}

export interface ProjectionResult<T extends ProjectionItem> {
  items: T[]
  status: ProjectionStatus
}

const intersects = (fromA: number, toA: number, fromB: number, toB: number) => {
  return !(toA < fromB || fromA > toB)
}

export type NodeIntersectsChangeFn = (node: SyntaxNodeRef) => boolean

export function updatePosition<T extends ProjectionItem>(item: T, transaction?: Transaction): T {
  if (!transaction) {
    return item
  }
  const { from, to } = item
  const newFrom = transaction.changes.mapPos(from)
  const newTo = transaction.changes.mapPos(to)
  const newLine = transaction.state.doc.lineAt(newFrom).number
  const newToLine = transaction.state.doc.lineAt(newTo).number

  if (newFrom === from && newTo === to && newLine === item.line && newToLine === item.toLine) {
    return item
  }

  return { ...item, from: newFrom, to: newTo, line: newLine, toLine: newToLine }
}

export type EnterNodeFn<T> = (
  state: EditorState,
  node: SyntaxNodeRef,
  items: T[],
  nodeIntersectsChange: NodeIntersectsChangeFn
) => boolean | void

/**
 * Calculates an updated projection of an editor state, reusing the previous
 * projection's items outside the changed range.
 */
export function getUpdatedProjection<T extends ProjectionItem>(
  state: EditorState,
  fromA: number,
  toA: number,
  fromB: number,
  toB: number,
  initialParse = false,
  enterNode: EnterNodeFn<T>,
  transaction?: Transaction,
  previousResult: ProjectionResult<T> = { items: [], status: ProjectionStatus.Pending }
): ProjectionResult<T> {
  const items: T[] =
    previousResult.status === ProjectionStatus.Complete
      ? previousResult.items.filter(item => !intersects(item.from, item.to, fromA, toA)).map(x => updatePosition(x, transaction))
      : []

  if (previousResult.status !== ProjectionStatus.Complete) {
    toB = state.doc.length
    fromB = 0
  }
  const tree = ensureSyntaxTree(state, toB, initialParse ? FIVE_HUNDRED_MS : TWENTY_MS)
  if (tree) {
    tree.iterate({
      from: fromB,
      to: toB,
      enter(node) {
        const nodeIntersectsChange = (n: SyntaxNodeRef) => intersects(n.from, n.to, fromB, toB)
        return enterNode(state, node, items, nodeIntersectsChange) as boolean | void
      },
      mode: IterMode.IgnoreMounts | IterMode.IgnoreOverlays,
    })
    return {
      status: ProjectionStatus.Complete,
      items: items.sort((a, b) => a.from - b.from),
    }
  } else if (previousResult.status !== ProjectionStatus.Pending) {
    return { status: ProjectionStatus.Partial, items: previousResult.items }
  } else {
    return { items: [], status: ProjectionStatus.Pending }
  }
}
