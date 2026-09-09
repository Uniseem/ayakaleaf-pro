/**
 * Operational transformation for text, as this deployment's server speaks it.
 *
 * Two people typing in one document produce edits against different versions
 * of it. Sending positions alone loses: by the time an edit arrives, the text
 * it was written against has moved. Transformation is how an edit is rewritten
 * to mean the same thing against text it has not seen.
 *
 * The model is the one the server implements: an operation is a list of
 * components, each an insert `{p, i}` or a delete `{p, d}` at a character
 * position in the whole document. Nothing here is novel -- it has to match the
 * server exactly or documents diverge, which is the one failure in a
 * collaborative editor that cannot be recovered by reloading.
 */

/** One insert or delete at a position. */
export type Component =
  | { p: number; i: string }
  | { p: number; d: string }

/** An operation is a list of components applied in order. */
export type Op = Component[]

export function isInsert(component: Component): component is { p: number; i: string } {
  return 'i' in component
}

export function lengthOf(component: Component): number {
  return isInsert(component) ? component.i.length : component.d.length
}

/** Applies an operation to text. */
export function apply(text: string, op: Op): string {
  let result = text
  for (const component of op) {
    if (isInsert(component)) {
      result = result.slice(0, component.p) + component.i + result.slice(component.p)
    } else {
      // The delete says what it removed; if the text there is not that, the
      // document has already diverged and going on would make it worse.
      const found = result.slice(component.p, component.p + component.d.length)
      if (found !== component.d) {
        throw new Error('That edit does not match the document.')
      }
      result = result.slice(0, component.p) + result.slice(component.p + component.d.length)
    }
  }
  return result
}

/**
 * Adds a component to an operation, merging it into the previous one when the
 * two are adjacent inserts or adjacent deletes.
 *
 * Not an optimisation. transformX below relies on knowing whether transforming
 * one component produced none, one, or several, and without merging it would
 * see "several" for a pair that is really one edit, and recurse where it
 * should not.
 */
function appendComponent(op: Op, component: Component): Op {
  if (lengthOf(component) === 0) {
    return op
  }
  const last = op[op.length - 1]
  if (!last) {
    return [...op, component]
  }

  if (isInsert(last) && isInsert(component)) {
    if (last.p <= component.p && component.p <= last.p + last.i.length) {
      const at = component.p - last.p
      const merged = last.i.slice(0, at) + component.i + last.i.slice(at)
      return [...op.slice(0, -1), { p: last.p, i: merged }]
    }
  } else if (!isInsert(last) && !isInsert(component)) {
    if (component.p <= last.p && last.p <= component.p + component.d.length) {
      const at = last.p - component.p
      const merged =
        component.d.slice(0, at) + last.d + component.d.slice(at)
      return [...op.slice(0, -1), { p: component.p, d: merged }]
    }
  }
  return [...op, component]
}

/**
 * Rewrites `op` so that it means the same thing after `other` has been applied.
 *
 * `side` decides what happens when two inserts land on the same position:
 * exactly one of the two clients must yield, or they would each place
 * themselves first and the documents would differ by a swap. 'left' goes
 * first.
 *
 * A one-against-one transform is the component rule below. Anything larger
 * goes through transformX, because transforming each component of one against
 * each component of the other in turn is *not* correct: the components of an
 * operation are positioned against the text its earlier components produced,
 * so transforming the second one has to account for what happened to the
 * first. Getting this wrong is not a rare edge case -- pasting over a
 * selection is a delete and an insert in one operation.
 */
export function transform(op: Op, other: Op, side: 'left' | 'right'): Op {
  if (other.length === 0) {
    return op
  }
  if (op.length === 1 && other.length === 1) {
    const first = op[0]
    const second = other[0]
    if (first && second) {
      return transformComponent(first, second, side)
    }
  }
  if (side === 'left') {
    return transformX(op, other)[0]
  }
  return transformX(other, op)[1]
}

/**
 * Transforms two whole operations against each other.
 *
 * Answers with the pair that converges: applying left then the new right
 * reaches the same text as applying right then the new left. Ported from the
 * server's implementation, which is the only definition that matters -- this
 * has to agree with it exactly.
 */
function transformX(leftOp: Op, rightOp: Op): [Op, Op] {
  let left = leftOp
  let newRight: Op = []

  for (const rc of rightOp) {
    let rightComponent: Component | null = rc
    let newLeft: Op = []

    let k = 0
    while (k < left.length) {
      const leftComponent = left[k]
      if (!leftComponent || !rightComponent) {
        break
      }
      for (const piece of transformComponent(leftComponent, rightComponent, 'left')) {
        newLeft = appendComponent(newLeft, piece)
      }
      const next = transformComponent(rightComponent, leftComponent, 'right')
      k++

      if (next.length === 1) {
        rightComponent = next[0] ?? null
      } else if (next.length === 0) {
        // Cancelled out entirely, so the rest of the left operation is
        // untouched by it.
        for (const rest of left.slice(k)) {
          newLeft = appendComponent(newLeft, rest)
        }
        rightComponent = null
      } else {
        // It split. The remainder of the left operation has to be transformed
        // against all the pieces, which is the same problem again.
        const [restLeft, restRight] = transformX(left.slice(k), next)
        for (const piece of restLeft) {
          newLeft = appendComponent(newLeft, piece)
        }
        for (const piece of restRight) {
          newRight = appendComponent(newRight, piece)
        }
        rightComponent = null
      }

      if (rightComponent === null) {
        break
      }
    }

    if (rightComponent !== null) {
      newRight = appendComponent(newRight, rightComponent)
    }
    left = newLeft
  }

  return [left, newRight]
}

/**
 * Transforms one component against one other.
 *
 * This is where the rules live, and every one of them has to match the
 * server's. A disagreement is two documents that drift apart with nobody
 * told.
 */
function transformComponent(
  component: Component,
  other: Component,
  side: 'left' | 'right'
): Op {
  if (isInsert(component)) {
    // An insert only ever moves; it never splits and never disappears.
    return [{ p: movePosition(component.p, other, side === 'right'), i: component.i }]
  }

  if (isInsert(other)) {
    // A delete with an insert landing inside it splits around the inserted
    // text, which was not part of what was being deleted.
    const out: Op = []
    let text = component.d
    if (component.p < other.p) {
      out.push({ p: component.p, d: text.slice(0, other.p - component.p) })
      text = text.slice(other.p - component.p)
    }
    if (text.length > 0) {
      // Positioned as if the piece above has already been applied, because
      // the components of one operation apply in order.
      out.push({ p: component.p + other.i.length, d: text })
    }
    return out
  }

  // Two deletes.
  const otherEnd = other.p + other.d.length
  if (component.p >= otherEnd) {
    return [{ p: component.p - other.d.length, d: component.d }]
  }
  if (component.p + component.d.length <= other.p) {
    return [component]
  }

  // They overlap. Only the part the other did not take is still there.
  let remaining = ''
  if (component.p < other.p) {
    remaining += component.d.slice(0, other.p - component.p)
  }
  if (component.p + component.d.length > otherEnd) {
    remaining += component.d.slice(otherEnd - component.p)
  }
  if (remaining === '') {
    return []
  }
  return [{ p: movePosition(component.p, other, false), d: remaining }]
}

/**
 * Where a position ends up after one component is applied.
 *
 * `insertAfter` is what breaks a tie between an insert at exactly this
 * position and this position itself.
 */
function movePosition(position: number, component: Component, insertAfter: boolean): number {
  if (isInsert(component)) {
    if (component.p < position || (component.p === position && insertAfter)) {
      return position + component.i.length
    }
    return position
  }
  if (position <= component.p) {
    return position
  }
  if (position <= component.p + component.d.length) {
    return component.p
  }
  return position - component.d.length
}

/**
 * Joins two operations into one that does what both did.
 *
 * Used to batch what somebody typed while an earlier edit was still in flight,
 * so that a fast typist sends a few operations rather than one per keystroke.
 */
export function compose(first: Op, second: Op): Op {
  // Correct without being clever: the components are already positioned
  // against the text the previous one produced, so concatenating them applies
  // in the same order and gives the same result. Merging adjacent components
  // would make the message smaller; it would also be a second place for the
  // position arithmetic to be wrong.
  return [...first, ...second]
}

/** Turns a change to the text into components, given both versions. */
export function diffToOp(before: string, after: string): Op {
  if (before === after) {
    return []
  }
  // The common prefix and suffix, which is all a single edit ever changes.
  // CodeMirror hands over one change at a time, so this is exact for typing
  // and good enough for a paste.
  let start = 0
  const limit = Math.min(before.length, after.length)
  while (start < limit && before[start] === after[start]) {
    start++
  }
  let end = 0
  while (
    end < limit - start &&
    before[before.length - 1 - end] === after[after.length - 1 - end]
  ) {
    end++
  }

  const removed = before.slice(start, before.length - end)
  const added = after.slice(start, after.length - end)

  const op: Op = []
  if (removed) {
    op.push({ p: start, d: removed })
  }
  if (added) {
    op.push({ p: start, i: added })
  }
  return op
}

/** Where a cursor at `position` ends up after an operation. */
export function transformPosition(position: number, op: Op): number {
  let moved = position
  for (const component of op) {
    if (isInsert(component)) {
      if (component.p <= moved) {
        moved += component.i.length
      }
    } else {
      if (component.p + component.d.length <= moved) {
        moved -= component.d.length
      } else if (component.p < moved) {
        moved = component.p
      }
    }
  }
  return moved
}
