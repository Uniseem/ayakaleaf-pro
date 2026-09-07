import { createServer } from '../../../../app/js/server.js'
import { promisify } from 'node:util'
import './MongoHelper.js'
import testLogRecorder from '@overleaf/logger/test-log-recorder.js'

export { db } from '../../../../app/js/mongodb.js'

let serverPromise = null

// Set CHAT_EXTERNAL=true to run this suite against a chat service that is
// already listening on port 3010 instead of starting the Node app in-process.
// That is how the Go port in services-go is held to the same contract: the
// tests, the client and the assertions are unchanged, only the implementation
// behind the port differs.
const EXTERNAL = process.env.CHAT_EXTERNAL === 'true'
const READY_TIMEOUT_MS = parseInt(process.env.CHAT_EXTERNAL_TIMEOUT_MS || '30000', 10)

async function waitForExternalService() {
  const deadline = Date.now() + READY_TIMEOUT_MS
  let lastError
  while (Date.now() < deadline) {
    try {
      const response = await fetch('http://127.0.0.1:3010/status')
      if (response.ok) {
        return
      }
      lastError = new Error(`status endpoint returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 250))
  }
  throw new Error(
    `chat service on port 3010 was not ready within ${READY_TIMEOUT_MS}ms: ${lastError}`
  )
}

export async function ensureRunning() {
  if (!serverPromise) {
    if (EXTERNAL) {
      serverPromise = waitForExternalService()
    } else {
      const { app } = await createServer()
      const startServer = promisify(app.listen.bind(app))
      serverPromise = startServer(3010, '127.0.0.1')
    }
  }
  return serverPromise
}

if (process.env.CI === 'true') {
  beforeEach('record error logs in junit', testLogRecorder)
}
