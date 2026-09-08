// Differential harness for the Go port of document-updater's RedisManager.
//
// It drives the real RedisManager, so the Go store can be checked against the
// implementation it replaces rather than against a second reading of the same
// source. The Go test writes with one and reads with the other, both ways
// round: a difference in how a document is serialised, hashed or keyed shows up
// as a mismatched field instead of as a corrupted document later.
//
// The service directory is passed as an argument, because require resolves
// relative to this file rather than to the working directory:
//   DOC_UPDATER_REDIS_HOST=... node redisharness.js /path/to/services/document-updater
'use strict'

const path = require('node:path')
const readline = require('node:readline')

const serviceDir = process.argv[2]
if (!serviceDir) {
  process.stderr.write('usage: node redisharness.js <service dir>\n')
  process.exit(2)
}
const RedisManager = require(path.join(serviceDir, 'app', 'js', 'RedisManager'))

async function run(cmd) {
  switch (cmd.op) {
    case 'put':
      await RedisManager.promises.putDocInMemory(
        cmd.projectId,
        cmd.docId,
        cmd.lines,
        cmd.version,
        cmd.ranges,
        cmd.resolvedCommentIds || [],
        cmd.pathname,
        cmd.projectHistoryId,
        cmd.historyRangesSupport || false
      )
      return {}

    case 'get':
      return { doc: await RedisManager.promises.getDoc(cmd.projectId, cmd.docId) }

    case 'update':
      await RedisManager.promises.updateDocument(
        cmd.projectId,
        cmd.docId,
        cmd.lines,
        cmd.version,
        cmd.appliedOps || [],
        cmd.ranges,
        cmd.updateMeta
      )
      return {}

    case 'getPreviousDocOps':
      return {
        ops: await RedisManager.promises.getPreviousDocOps(
          cmd.docId,
          cmd.start,
          cmd.end
        ),
      }

    case 'remove':
      await RedisManager.promises.removeDocFromMemory(cmd.projectId, cmd.docId)
      return {}

    case 'docIdsInProject':
      return {
        docIds: await RedisManager.promises.getDocIdsInProject(cmd.projectId),
      }

    default:
      throw new Error(`unknown op ${cmd.op}`)
  }
}

const rl = readline.createInterface({ input: process.stdin, terminal: false })
const queue = []
let running = false

async function drain() {
  if (running) return
  running = true
  while (queue.length) {
    const line = queue.shift()
    let result
    try {
      result = await run(JSON.parse(line))
    } catch (err) {
      result = { error: err.message || String(err) }
    }
    process.stdout.write(JSON.stringify(result) + '\n')
  }
  running = false
}

rl.on('line', line => {
  if (!line.trim()) return
  queue.push(line)
  drain()
})
rl.on('close', () => {
  // The Redis client keeps the event loop alive.
  const finish = () => {
    if (queue.length || running) return setTimeout(finish, 10)
    RedisManager.rclient.disconnect()
  }
  finish()
})
