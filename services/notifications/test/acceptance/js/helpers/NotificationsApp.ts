import logger from '@overleaf/logger'
import type { Server } from 'node:http'

export const HOSTNAME = '127.0.0.1'
export const PORT = 3042
export const BASE_URL = `http://${HOSTNAME}:${PORT}`

// Set NOTIFICATIONS_EXTERNAL=true to run the acceptance suites against a
// notifications service that is already listening on PORT, rather than
// starting the Node app in-process. The Go port in services-go is verified
// this way: same tests, same assertions, different implementation behind the
// port.
const EXTERNAL = process.env.NOTIFICATIONS_EXTERNAL === 'true'

// Kept under vitest's 10s default hookTimeout, since ensureRunning is awaited
// from beforeAll.
const READY_TIMEOUT_MS = parseInt(
  process.env.NOTIFICATIONS_EXTERNAL_TIMEOUT_MS || '8000',
  10
)

let runAppPromise: Promise<void> | null = null

async function waitForService(): Promise<void> {
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
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(
    `notifications service on port ${PORT} was not ready within ${READY_TIMEOUT_MS}ms: ${lastError}`
  )
}

async function startInProcess(): Promise<void> {
  // Imported lazily so that running against an external service does not pull
  // in the Node app and open its Mongo connection.
  const { default: app } = await import('../../../../app.ts')

  await new Promise<void>((resolve, reject) => {
    const server: Server = app.listen(PORT, HOSTNAME, () => {
      logger.info({ port: PORT, hostname: HOSTNAME }, 'notifications running in dev mode')
      resolve()
    })
    server.on('error', (error: NodeJS.ErrnoException) => {
      if (error.code === 'EADDRINUSE') {
        // Each test file gets its own module graph, so this helper is
        // instantiated once per file and the promise above cannot be shared
        // between them. Whichever file gets there first starts the service;
        // the rest just wait for it.
        resolve(waitForService())
        return
      }
      reject(error)
    })
  })
}

export async function ensureRunning(): Promise<void> {
  if (!runAppPromise) {
    runAppPromise = EXTERNAL ? waitForService() : startInProcess()
  }
  await runAppPromise
}
