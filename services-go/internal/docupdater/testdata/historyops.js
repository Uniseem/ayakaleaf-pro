// A harness around the real RangesManager.applyUpdate, so the Go port can be
// compared against it over the same updates.
//
// It reads one JSON scenario per line on stdin and writes one JSON result per
// line on stdout, in the same order.
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const serviceDir = process.argv[2]
if (!serviceDir) {
  process.stderr.write('usage: historyops.js <document-updater dir>\n')
  process.exit(2)
}
const RangesManager = require(path.join(serviceDir, 'app', 'js', 'RangesManager'))

const out = []
readline
  .createInterface({ input: process.stdin, crlfDelay: Infinity })
  .on('line', line => {
    if (line === '') return
    const scenario = JSON.parse(line)
    try {
      const result = RangesManager.applyUpdate(
        'p',
        'd',
        { changes: scenario.changes || [], comments: scenario.comments || [] },
        scenario.updates,
        scenario.newDocLines,
        { historyRangesSupport: scenario.historyRangesSupport }
      )
      out.push(
        JSON.stringify({
          historyUpdates: result.historyUpdates,
          newRanges: result.newRanges,
          removedChangeIds: result.removedChangeIds,
          rangesWereCollapsed: result.rangesWereCollapsed,
        })
      )
    } catch (err) {
      out.push(JSON.stringify({ error: String(err && err.message) }))
    }
  })
  .on('close', () => {
    process.stdout.write(out.join('\n') + '\n')
  })
