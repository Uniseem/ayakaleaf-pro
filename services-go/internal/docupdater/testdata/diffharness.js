// A harness around the real DiffCodec, so the Go port can be compared against
// it over the same scenarios.
//
// It reads one JSON scenario per line on stdin and writes one JSON result per
// line on stdout, in the same order.
//
//   node diffharness.js /path/to/services/document-updater
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const serviceDir = process.argv[2]
if (!serviceDir) {
  process.stderr.write('usage: diffharness.js <document-updater dir>\n')
  process.exit(2)
}
const DiffCodec = require(path.join(serviceDir, 'app', 'js', 'DiffCodec'))

const out = []
readline
  .createInterface({ input: process.stdin, crlfDelay: Infinity })
  .on('line', line => {
    if (line === '') return
    const scenario = JSON.parse(line)
    try {
      out.push(
        JSON.stringify({
          op: DiffCodec.diffAsShareJsOp(scenario.before, scenario.after),
        })
      )
    } catch (err) {
      out.push(JSON.stringify({ error: String(err && err.message) }))
    }
  })
  .on('close', () => {
    process.stdout.write(out.join('\n') + '\n')
  })
