require('./MongoHelper')

// Set DOCUPDATER_EXTERNAL=true to run this suite against a document-updater
// that is already listening on the configured port instead of starting the
// Node app in-process. That is how the Go port in services-go is held to the
// same contract: the tests, the HTTP client and the assertions are unchanged,
// only the implementation behind the port differs.
const EXTERNAL = process.env.DOCUPDATER_EXTERNAL === 'true'
const READY_TIMEOUT_MS = parseInt(
  process.env.DOCUPDATER_EXTERNAL_TIMEOUT_MS || '30000',
  10
)

const HOST = '127.0.0.1'
const PORT = 3003

function startApp() {
  // Required lazily so that a run against an external service never loads the
  // Node app or opens its Redis and Mongo connections.
  const app = require('../../../../app')
  return new Promise((resolve, reject) => {
    app.listen(PORT, HOST, error => {
      if (error) {
        reject(error)
      } else {
        resolve()
      }
    })
  })
}

async function waitForExternalService() {
  const deadline = Date.now() + READY_TIMEOUT_MS
  let lastError
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://${HOST}:${PORT}/status`)
      if (response.ok) {
        return
      }
      lastError = new Error(`status endpoint returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(
    `document-updater on port ${PORT} was not ready within ${READY_TIMEOUT_MS}ms: ${lastError}`
  )
}

let appStartedPromise

async function ensureRunning() {
  if (!appStartedPromise) {
    appStartedPromise = EXTERNAL ? waitForExternalService() : startApp()
  }
  await appStartedPromise
}

module.exports = {
  ensureRunning,
}
