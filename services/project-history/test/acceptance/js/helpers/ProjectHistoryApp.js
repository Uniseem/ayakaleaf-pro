import './MongoHelper.js'

// Set PROJECT_HISTORY_EXTERNAL=true to run this suite against a project-history
// that is already listening on the configured port instead of starting the Node
// app in-process. That is how the Go port in services-go is held to the same
// contract: the tests, the HTTP client and the assertions are unchanged, only
// the implementation behind the port differs.
//
// The mocks this suite sets up have to reach that process too, which is what
// helpers/MockServices.js is for.
const EXTERNAL = process.env.PROJECT_HISTORY_EXTERNAL === 'true'
const READY_TIMEOUT_MS = parseInt(
  process.env.PROJECT_HISTORY_EXTERNAL_TIMEOUT_MS || '30000',
  10
)

const HOST = '127.0.0.1'
const PORT = 3054

let running = false
let initPromise = null

async function initialize() {
  try {
    // Imported lazily so that a run against an external service never loads the
    // Node app or opens its Mongo and Redis connections.
    const { app } = await import('../../../../app/js/server.js')
    const { mongoClient } = await import('../../../../app/js/mongodb.js')

    await new Promise((resolve, reject) => {
      app.listen(PORT, HOST, error => {
        if (error) return reject(error)
        resolve()
      })
    })

    await mongoClient.connect()

    running = true
  } catch (error) {
    initPromise = null
    throw error
  }
}

async function waitForExternalService() {
  const deadline = Date.now() + READY_TIMEOUT_MS
  let lastError
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://${HOST}:${PORT}/status`)
      if (response.ok) {
        running = true
        return
      }
      lastError = new Error(`status endpoint returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  initPromise = null
  throw new Error(
    `project-history on port ${PORT} was not ready within ${READY_TIMEOUT_MS}ms: ${lastError}`
  )
}

export async function ensureRunning() {
  if (running) {
    return
  }

  if (initPromise) {
    return await initPromise
  }

  initPromise = EXTERNAL ? waitForExternalService() : initialize()
  return await initPromise
}
