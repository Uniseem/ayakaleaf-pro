import Settings from '@overleaf/settings'
import './MongoHelper.js'

// Set DOCSTORE_EXTERNAL=true to run this suite against a docstore that is
// already listening on the configured port instead of starting the Node app
// in-process. That is how the Go port in services-go is held to the same
// contract: the tests, the HTTP client and the assertions are unchanged, only
// the implementation behind the port differs.
const EXTERNAL = process.env.DOCSTORE_EXTERNAL === 'true'
const READY_TIMEOUT_MS = parseInt(
  process.env.DOCSTORE_EXTERNAL_TIMEOUT_MS || '30000',
  10
)

const { host, port } = Settings.internal.docstore

async function startApp() {
  // Imported lazily so that a run against an external service never loads the
  // Node app or opens its Mongo connection.
  const { default: app } = await import('../../../../app.js')
  return await new Promise((resolve, reject) => {
    app.listen(port, host, error => {
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
      const response = await fetch(`http://${host}:${port}/status`)
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
    `docstore on port ${port} was not ready within ${READY_TIMEOUT_MS}ms: ${lastError}`
  )
}

let appStartedPromise

async function ensureRunning() {
  if (!appStartedPromise) {
    appStartedPromise = EXTERNAL ? waitForExternalService() : startApp()
  }
  await appStartedPromise
}

export default {
  ensureRunning,
}
