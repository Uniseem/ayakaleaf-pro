// A harness around the real overleaf-editor-core TextOperation, so the Go port
// can be compared against it over the same operations.
//
// It reads one JSON scenario per line on stdin and writes one JSON result per
// line on stdout, in the same order.
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const libraryDir = process.argv[2]
if (!libraryDir) {
  process.stderr.write('usage: harness.js <overleaf-editor-core dir>\n')
  process.exit(2)
}
const { TextOperation, TrackedChangeList } = require(libraryDir)

const out = []
readline
  .createInterface({ input: process.stdin, crlfDelay: Infinity })
  .on('line', line => {
    if (line === '') return
    const scenario = JSON.parse(line)
    const answer = {}
    try {
      // A round trip: read the operation and write it back. Anything the two
      // implementations normalise differently shows up here.
      const first = TextOperation.fromJSON({ textOperation: scenario.first })
      answer.roundTrip = first.toJSON()
      answer.baseLength = first.baseLength
      answer.targetLength = first.targetLength

      if (scenario.length !== undefined) {
        try {
          answer.appliedLength = first.applyToLength(scenario.length)
        } catch (err) {
          answer.applyError = String(err && err.message)
        }
      }

      // The tracked changes have to move with the operation, and where they
      // end up is the thing a restored version is read through.
      if (Array.isArray(scenario.trackedChanges)) {
        try {
          const list = TrackedChangeList.fromRaw(scenario.trackedChanges)
          list.applyTextOperation(first)
          answer.trackedChanges = list.toRaw()
        } catch (err) {
          answer.trackedChangesError = String(err && err.message)
        }
      }

      if (Array.isArray(scenario.second)) {
        try {
          const second = TextOperation.fromJSON({
            textOperation: scenario.second,
          })
          answer.composed = first.compose(second).toJSON()
        } catch (err) {
          answer.composeError = String(err && err.message)
        }
      }
    } catch (err) {
      answer.error = String(err && err.message)
    }
    out.push(JSON.stringify(answer))
  })
  .on('close', () => {
    process.stdout.write(out.join('\n') + '\n')
  })
