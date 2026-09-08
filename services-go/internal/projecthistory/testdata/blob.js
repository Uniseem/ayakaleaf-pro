// A harness around the real HistoryBlobTranslator, HashManager and Versions,
// so the Go port can be compared against them.
//
// It reads one JSON scenario per line on stdin and writes one JSON result per
// line on stdout, in the same order.
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const serviceDir = process.argv[2]
if (!serviceDir) {
  process.stderr.write('usage: blob.js <project-history dir>\n')
  process.exit(2)
}

async function main() {
  const HistoryBlobTranslator = await import(
    path.join(serviceDir, 'app', 'js', 'HistoryBlobTranslator.js')
  )
  const HashManager = await import(
    path.join(serviceDir, 'app', 'js', 'HashManager.js')
  )
  const Versions = await import(path.join(serviceDir, 'app', 'js', 'Versions.js'))

  const out = []
  const rl = readline.createInterface({
    input: process.stdin,
    crlfDelay: Infinity,
  })
  for await (const line of rl) {
    if (line === '') continue
    const scenario = JSON.parse(line)
    const result = {}
    try {
      const ranges = HistoryBlobTranslator.createRangeBlobDataFromUpdate(
        scenario.update
      )
      // undefined and null are different answers here: the first says there is
      // nothing to store, and JSON has only the second.
      result.ranges = ranges === undefined ? null : ranges
      result.hash = HashManager._getBlobHashFromString(
        scenario.update.docLines ?? ''
      )
      if (scenario.versions) {
        result.compare = scenario.versions.map(([a, b]) =>
          Versions.compare(a, b)
        )
      }
    } catch (err) {
      result.error = String(err && err.message)
    }
    out.push(JSON.stringify(result))
  }
  process.stdout.write(out.join('\n') + '\n')
}

main().catch(err => {
  process.stderr.write(String(err) + '\n')
  process.exit(1)
})
