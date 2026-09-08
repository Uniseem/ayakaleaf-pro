// A harness around the real UpdateTranslator, so the Go port can be compared
// against it over the same updates.
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const serviceDir = process.argv[2]
if (!serviceDir) {
  process.stderr.write('usage: translator.js <project-history dir>\n')
  process.exit(2)
}

async function main() {
  const UpdateTranslator = await import(
    path.join(serviceDir, 'app', 'js', 'UpdateTranslator.js')
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
      const changes = UpdateTranslator.convertToChanges(
        'p',
        scenario.updates
      ).map(change => change.toRaw())
      out.push(JSON.stringify({ changes }))
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
