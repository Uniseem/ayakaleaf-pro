// Differential harness for the Go port of the ShareJS text type.
//
// It reads one scenario per line on stdin and writes one result per line on
// stdout, using the real text.js the Node service runs. The Go test generates
// the scenarios and compares the answers, so any disagreement between the two
// implementations shows up as a concrete pair of operations rather than as a
// corrupted document six months from now.
//
// Usage: node harness.js /path/to/sharejs/types/text.js
'use strict'

const readline = require('node:readline')

const textPath = process.argv[2]
if (!textPath) {
  process.stderr.write('usage: node harness.js <path to sharejs text.js>\n')
  process.exit(2)
}
const text = require(textPath)

function run(scenario) {
  const out = {}
  try {
    // Transform is destructive in places, so each call gets its own copy.
    out.aLeft = text.transform(
      JSON.parse(JSON.stringify(scenario.a)),
      JSON.parse(JSON.stringify(scenario.b)),
      'left'
    )
    out.bRight = text.transform(
      JSON.parse(JSON.stringify(scenario.b)),
      JSON.parse(JSON.stringify(scenario.a)),
      'right'
    )
    out.docA = text.apply(scenario.snapshot, JSON.parse(JSON.stringify(scenario.a)))
    out.docB = text.apply(scenario.snapshot, JSON.parse(JSON.stringify(scenario.b)))
    out.afterAThenB = text.apply(out.docA, out.bRight)
    out.afterBThenA = text.apply(out.docB, out.aLeft)
  } catch (err) {
    out.error = err.message || String(err)
  }
  return out
}

const rl = readline.createInterface({ input: process.stdin, terminal: false })
rl.on('line', line => {
  if (!line.trim()) return
  process.stdout.write(JSON.stringify(run(JSON.parse(line))) + '\n')
})
