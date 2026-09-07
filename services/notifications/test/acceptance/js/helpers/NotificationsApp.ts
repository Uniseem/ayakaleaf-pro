import logger from '@overleaf/logger'

export const HOSTNAME = '127.0.0.1'
export const PORT = 3042
export const BASE_URL = `http://${HOSTNAME}:${PORT}`

// Set NOTIFICATIONS_EXTERNAL=true to run the acceptance suites against a
// notifications service that is already listening on PORT, rather than
// starting the Node app in-process. The Go port in services-go is verified
// this way: same tests, same assertions, different implementation behind the
// port.
const EXTERNAL = process.env.NOTIFICATIONS_EXTERNAL === 'true'
const READY_TIMEOUT_MS = parseInt(
  process.env.NOTIFICATIONS_EXTERNAL_TIMEOUT_MS || '30000',
  10
)

let runAppPromise: Promise<void> | null = null

async function waitForExternalService(): Promise<void> {
  const deadline = Date.now() + READY_TIMEOUT_MS
  let lastError: unknown
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${BASE_URL}/status`)
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
    `notifications service on port ${PORT} was not ready within ${READY_TIMEOUT_MS}ms: ${lastError}`
  )
}

async function startInProcess(): Promise<void> {
  // Imported lazily so that running against an external service does not pull
  // in the Node app and open its Mongo connection.
  const { default: app } = await import('../../../../app.ts')
  await new Promise<void>(resolve => {
    app.listen(PORT, HOSTNAME, () => {
      logger.info({ port: PORT, hostname: HOSTNAME }, 'notifications running in dev mode')
      resolve()
    })
  })
}

export async function ensureRunning(): Promise<void> {
  if (!runAppPromise) {
    runAppPromise = EXTERNAL ? waitForExternalService() : startInProcess()
  }
  await runAppPromise
}
