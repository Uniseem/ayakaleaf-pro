import { exec } from 'node:child_process'
import { promisify } from 'node:util'
import type { Server } from 'node:http'
import { EXTERNAL, HOSTNAME, PORT, waitForService } from './helpers/NotificationsApp.ts'

// Global setup for the acceptance suites.
//
// Vitest gives each test file its own module graph, so anything a test file
// does at import time happens once per file. Running the migrations and
// starting the service from there meant doing both twice, concurrently: the
// two migration runs raced on the migrations collection's unique index, and
// the two servers raced for port 3042. Global setup runs exactly once for the
// whole run, which is what both of those need.

const MIGRATION_TIMEOUT_MS = 60_000

async function runMigrations(): Promise<void> {
  await promisify(exec)(
    'cd ../../tools/migrations && npm run migrations -- migrate -t server-ce',
    { timeout: MIGRATION_TIMEOUT_MS }
  )
}

async function startService(): Promise<Server> {
  // Imported here rather than at module scope so that a run against an
  // external service never loads the Node app or opens its Mongo connection.
  const { default: app } = await import('../../../app.ts')
  return await new Promise<Server>((resolve, reject) => {
    const server: Server = app.listen(PORT, HOSTNAME, () => resolve(server))
    server.on('error', reject)
  })
}

export async function setup(): Promise<() => Promise<void>> {
  await runMigrations()

  if (EXTERNAL) {
    await waitForService()
    return async () => {}
  }

  const server = await startService()
  await waitForService()
  return async () => {
    await new Promise<void>(resolve => server.close(() => resolve()))
    // app.ts opens a Mongo connection at import time. Left open, it keeps the
    // Vite server from exiting and the run ends in a close timeout.
    const { mongoClient } = await import('../../../app/js/mongodb.js')
    await mongoClient.close()
  }
}
