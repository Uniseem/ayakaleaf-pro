// Differential harness for the Go port of ranges-tracker.
//
// It drives the real RangesTracker so the port can be checked against the
// implementation it replaces. The Go test generates the scenarios; this applies
// them and hands back the markers that resulted, and any error raised on the
// way.
//
// Usage: node harness.js /path/to/libraries/ranges-tracker
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const libraryDir = process.argv[2]
if (!libraryDir) {
  process.stderr.write('usage: node harness.js <path to ranges-tracker>\n')
  process.exit(2)
}
const RangesTracker = require(path.join(libraryDir, 'index.cjs'))

function run(scenario) {
  const tracker = new RangesTracker(
    scenario.changes || [],
    scenario.comments || []
  )
  tracker.setIdSeed(scenario.idSeed)

  try {
    for (const step of scenario.steps) {
      tracker.track_changes = step.trackChanges
      if (step.idSeed) tracker.setIdSeed(step.idSeed)
      tracker.applyOp(step.op, step.metadata)
    }
  } catch (err) {
    return { error: err.message || String(err) }
  }

  let validationError = null
  if (scenario.text != null) {
    try {
      tracker.validate(scenario.text)
    } catch (err) {
      validationError = err.message || String(err)
    }
  }

  return {
    changes: tracker.changes,
    comments: tracker.comments,
    validationError,
  }
}

const rl = readline.createInterface({ input: process.stdin, terminal: false })
rl.on('line', line => {
  if (!line.trim()) return
  process.stdout.write(JSON.stringify(run(JSON.parse(line))) + '\n')
})
