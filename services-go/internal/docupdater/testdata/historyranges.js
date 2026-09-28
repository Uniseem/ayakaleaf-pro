// A harness around the real HistoryConversions, so the Go port can be compared
// against it over the same markers.
//
// It reads one JSON scenario per line on stdin and writes one JSON result per
// line on stdout, in the same order.
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const serviceDir = process.argv[2]
if (!serviceDir) {
  process.stderr.write('usage: historyranges.js <document-updater dir>\n')
  process.exit(2)
}
const HistoryConversions = require(
  path.join(serviceDir, 'app', 'js', 'HistoryConversions')
)
const { addTrackedDeletesToContent } = require(
  path.join(serviceDir, 'app', 'js', 'Utils')
)

const out = []
readline
  .createInterface({ input: process.stdin, crlfDelay: Infinity })
  .on('line', line => {
    if (line === '') return
    const scenario = JSON.parse(line)
    // The callers pass `ranges.changes ?? []`; the harness does the same so a
    // scenario with no markers is not a different question.
    scenario.changes = scenario.changes || []
    scenario.comments = scenario.comments || []
    try {
      out.push(
        JSON.stringify({
          ranges: HistoryConversions.toHistoryRanges({
            changes: scenario.changes,
            comments: scenario.comments,
          }),
          content: addTrackedDeletesToContent(
            scenario.content,
            scenario.changes
          ),
        })
      )
    } catch (err) {
      out.push(JSON.stringify({ error: String(err && err.message) }))
    }
  })
  .on('close', () => {
    process.stdout.write(out.join('\n') + '\n')
  })
