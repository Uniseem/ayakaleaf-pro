// A harness around the real UpdateCompressor, so the Go port can be compared
// against it over the same queues.
//
// It reads one JSON scenario per line on stdin and writes one JSON result per
// line on stdout, in the same order.
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const serviceDir = process.argv[2]
if (!serviceDir) {
  process.stderr.write('usage: compressor.js <project-history dir>\n')
  process.exit(2)
}

async function main() {
  const UpdateCompressor = await import(
    path.join(serviceDir, 'app', 'js', 'UpdateCompressor.js')
  )

  const out = []
  const rl = readline.createInterface({
    input: process.stdin,
    crlfDelay: Infinity,
  })
  for await (const line of rl) {
    if (line === '') continue
    const scenario = JSON.parse(line)
    try {
      out.push(
        JSON.stringify({
          compressed: UpdateCompressor.compressRawUpdates(scenario.updates),
        })
      )
    } catch (err) {
      out.push(JSON.stringify({ error: String(err && err.message) }))
    }
  }
  process.stdout.write(out.join('\n') + '\n')
}

main().catch(err => {
  process.stderr.write(String(err) + '\n')
  process.exit(1)
})
