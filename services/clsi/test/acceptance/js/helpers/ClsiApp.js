import app from '../../../../app.js'
import Settings from '@overleaf/settings'
import { fetchNothing } from '@overleaf/fetch-utils'
import testLogRecorder from '@overleaf/logger/test-log-recorder.js'

// The suite normally starts the service inside the test process. Setting
// CLSI_EXTERNAL points it at whatever is already listening on the port
// instead, which is how the same tests are used to judge a different
// implementation of the same service.
const external = process.env.CLSI_EXTERNAL === 'true'

function startApp() {
  return new Promise((resolve, reject) => {
    app.listen(
      Settings.internal.clsi.port,
      Settings.internal.clsi.host,
      error => {
        if (error) {
          reject(error)
        } else {
          resolve()
        }
      }
    )
  })
}

// waitForExternalService waits for something to answer on the port. It is not
// a health check: a service that answers at all is one the suite can start
// against, and what it answers is what the suite is there to find out.
async function waitForExternalService() {
  const url = `${Settings.apis.clsi.url}/status`
  const deadline = Date.now() + 30_000
  let lastError
  while (Date.now() < deadline) {
    try {
      await fetchNothing(url, { signal: AbortSignal.timeout(1000) })
      return
    } catch (error) {
      lastError = error
      await new Promise(resolve => setTimeout(resolve, 250))
    }
  }
  throw new Error(
    `no clsi answering on ${url}: ${lastError && lastError.message}`
  )
}

let appStartedPromise

async function ensureRunning() {
  if (!appStartedPromise) {
    appStartedPromise = external ? waitForExternalService() : startApp()
  }
  await appStartedPromise
}

if (process.env.CI === 'true') {
  beforeEach('record error logs in junit', testLogRecorder)
}

export default {
  ensureRunning,
}
