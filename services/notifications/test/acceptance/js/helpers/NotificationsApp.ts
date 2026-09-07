export const HOSTNAME = '127.0.0.1'
export const PORT = 3042
export const BASE_URL = `http://${HOSTNAME}:${PORT}`

// Set NOTIFICATIONS_EXTERNAL=true to run the acceptance suites against a
// notifications service that is already listening on PORT, rather than
// starting the Node app. The Go port in services-go is verified this way:
// same tests, same assertions, different implementation behind the port.
export const EXTERNAL = process.env.NOTIFICATIONS_EXTERNAL === 'true'

const READY_TIMEOUT_MS = parseInt(
  process.env.NOTIFICATIONS_EXTERNAL_TIMEOUT_MS || '30000',
  10
)

/** Polls /status until the service answers, or the timeout expires. */
export async function waitForService(
  timeoutMs: number = READY_TIMEOUT_MS
): Promise<void> {
  const deadline = Date.now() + timeoutMs
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
    `notifications service on port ${PORT} was not ready within ${timeoutMs}ms: ${lastError}`
  )
}
