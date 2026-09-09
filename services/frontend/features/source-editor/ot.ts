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
 * Rewrites `op` to apply after `other` has been applied.
 *
 * `side` decides what happens when two inserts land on the same position:
 * exactly one of the two clients must yield, or they would each place
 * themselves first and the documents would differ by a swap. 'left' goes
 * first.
 */
export function transform(op: Op, other: Op, side: 'left' | 'right'): Op {
  let result = op
  for (const component of other) {
    result = transformAgainst(result, component, side)
  }
  return result
}

function transformAgainst(op: Op, other: Component, side: 'left' | 'right'): Op {
  const out: Op = []
  for (const component of op) {
    out.push(...transformComponent(component, other, side))
  }
  return out
}

function transformComponent(
  component: Component,
  other: Component,
  side: 'left' | 'right'
): Op {
  if (isInsert(other)) {
    const at = other.p
    const shift = other.i.length

    if (isInsert(component)) {
      // A tie is broken by side, and only a tie: the whole point is that both
      // clients decide the same way.
      const after =
        component.p > at || (component.p === at && side === 'right')
      return [{ p: after ? component.p + shift : component.p, i: component.i }]
    }

    // A delete with an insert landing inside it splits into two: the inserted
    // text was not part of what was being deleted and must survive.
    if (at <= component.p) {
      return [{ p: component.p + shift, d: component.d }]
    }
    if (at >= component.p + component.d.length) {
      return [component]
    }
    const cut = at - component.p
    return [
      { p: component.p, d: component.d.slice(0, cut) },
      { p: component.p + shift + cut, d: component.d.slice(cut) },
    ]
  }

  // `other` is a delete.
  const at = other.p
  const gone = other.d.length

  if (isInsert(component)) {
    if (component.p <= at) {
      return [component]
    }
    if (component.p >= at + gone) {
      return [{ p: component.p - gone, i: component.i }]
    }
    // Inserted into text that has since been deleted: it goes where that text
    // was. Keeping it is right -- somebody typed it.
    return [{ p: at, i: component.i }]
  }

  // Two deletes. Whatever they both removed is already gone, so this one keeps
  // only the part the other did not take.
  const start = Math.max(component.p, at)
  const end = Math.min(component.p + component.d.length, at + gone)
  if (start >= end) {
    // No overlap.
    return [
      {
        p: component.p > at ? component.p - gone : component.p,
        d: component.d,
      },
    ]
  }
  const before = component.d.slice(0, start - component.p)
  const after = component.d.slice(end - component.p)
  const remaining = before + after
  if (remaining === '') {
    return []
  }
  return [{ p: Math.min(component.p, at), d: remaining }]
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
