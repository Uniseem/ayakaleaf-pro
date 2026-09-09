/**
 * Convergence, checked by brute force.
 *
 * The property an OT implementation lives or dies by (TP1): for any two
 * operations made against the same document, applying one and then the
 * transformed other must reach the same text whichever way round you do it.
 * When it does not, two people editing one file end up with different
 * documents and neither is told.
 *
 * Random operations rather than hand-picked cases, because the failures are in
 * the boundaries -- an insert exactly at the end of a delete, two deletes that
 * overlap by one character -- and those are what nobody thinks to write down.
 *
 * Run:
 *   npx tsc features/source-editor/ot.ts --outDir /tmp/ot --module esnext  *     --target es2022 --moduleResolution bundler
 *   cp features/source-editor/ot.test.mjs /tmp/ot && node /tmp/ot/ot.test.mjs
 */

import assert from 'node:assert/strict'
import { apply, transform, diffToOp, compose } from './ot.js'

const ALPHABET = 'abcdefgh'

function randomInt(limit) {
  return Math.floor(Math.random() * limit)
}

/** A random single-component operation against `text`. */
function randomComponent(text) {
  if (text.length === 0 || Math.random() < 0.5) {
    const at = randomInt(text.length + 1)
    const length = 1 + randomInt(3)
    let inserted = ''
    for (let i = 0; i < length; i++) {
      inserted += ALPHABET[randomInt(ALPHABET.length)]
    }
    return { p: at, i: inserted }
  }
  const at = randomInt(text.length)
  const length = 1 + randomInt(Math.min(4, text.length - at))
  return { p: at, d: text.slice(at, at + length) }
}

/** A random operation of one or two components. */
function randomOp(text) {
  const op = []
  let current = text
  const count = 1 + randomInt(4)
  for (let i = 0; i < count; i++) {
    const component = randomComponent(current)
    op.push(component)
    current = apply(current, [component])
  }
  return op
}

let checked = 0
let composed = 0

for (let round = 0; round < 200000; round++) {
  const length = randomInt(20)
  let text = ''
  for (let i = 0; i < length; i++) {
    text += ALPHABET[randomInt(ALPHABET.length)]
  }

  const left = randomOp(text)
  const right = randomOp(text)

  // left then right', against right then left'
  const leftFirst = apply(apply(text, left), transform(right, left, 'right'))
  const rightFirst = apply(apply(text, right), transform(left, right, 'left'))

  assert.equal(
    leftFirst,
    rightFirst,
    `did not converge\n  text:  ${JSON.stringify(text)}\n` +
      `  left:  ${JSON.stringify(left)}\n` +
      `  right: ${JSON.stringify(right)}\n` +
      `  left first:  ${JSON.stringify(leftFirst)}\n` +
      `  right first: ${JSON.stringify(rightFirst)}`
  )
  checked++
}

// compose: doing both in one operation must equal doing them in turn.
for (let round = 0; round < 5000; round++) {
  const length = randomInt(20)
  let text = ''
  for (let i = 0; i < length; i++) {
    text += ALPHABET[randomInt(ALPHABET.length)]
  }
  const first = randomOp(text)
  const middle = apply(text, first)
  const second = randomOp(middle)

  assert.equal(
    apply(text, compose(first, second)),
    apply(middle, second),
    'compose did not match applying both'
  )
  composed++
}

// diffToOp must describe the change it was given.
for (let round = 0; round < 5000; round++) {
  const length = randomInt(20)
  let before = ''
  for (let i = 0; i < length; i++) {
    before += ALPHABET[randomInt(ALPHABET.length)]
  }
  const after = apply(before, randomOp(before))
  assert.equal(
    apply(before, diffToOp(before, after)),
    after,
    `diffToOp wrong for ${JSON.stringify(before)} -> ${JSON.stringify(after)}`
  )
}

console.log(`converged on ${checked} random pairs`)
console.log(`compose correct on ${composed}`)
console.log('diffToOp correct on 5000')
