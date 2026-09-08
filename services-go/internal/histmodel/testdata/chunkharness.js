// A harness around the real Chunk and Snapshot, so the Go port can be compared
// against them over the same chunks.
//
// It reads one JSON chunk per line on stdin and writes one JSON result per line
// on stdout, in the same order.
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const libraryDir = process.argv[2]
if (!libraryDir) {
  process.stderr.write('usage: chunkharness.js <overleaf-editor-core dir>\n')
  process.exit(2)
}
const { Chunk } = require(libraryDir)

const out = []
readline
  .createInterface({ input: process.stdin, crlfDelay: Infinity })
  .on('line', line => {
    if (line === '') return
    const scenario = JSON.parse(line)
    const answer = {}
    try {
      const chunk = Chunk.fromRaw(scenario.chunk)
      // The round trip: anything the two normalise differently shows up here.
      answer.roundTrip = chunk.toRaw()
      answer.endVersion = chunk.getEndVersion()

      // And the project as it was at each version the chunk covers, which is
      // what a reader of the history actually asks for.
      answer.snapshots = []
      for (let v = chunk.getStartVersion(); v <= chunk.getEndVersion(); v++) {
        try {
          const snapshot = chunk.getSnapshot().clone()
          const changes = chunk.getChanges().slice(0, v - chunk.getStartVersion())
          for (const change of changes) {
            change.applyTo(snapshot)
          }
          answer.snapshots.push(snapshot.toRaw())
        } catch (err) {
          answer.snapshots.push({ error: String(err && err.message) })
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
