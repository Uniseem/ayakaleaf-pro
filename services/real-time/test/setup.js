import * as chai from 'chai'
import sinon from 'sinon'
import chaiAsPromised from 'chai-as-promised'
import sinonChai from 'sinon-chai'
import Settings from '@overleaf/settings'

// Set REALTIME_EXTERNAL=true to run this suite against a real-time that is
// already listening on the configured port instead of starting the Node app
// in-process. That is how the Go port in services-go is held to the same
// contract: the tests, the socket.io client and the assertions are unchanged,
// only the implementation behind the port differs.
//
// This suite can do that because it was always a black-box one -- it connects
// over a real socket, seeds sessions straight into Redis and mocks web and
// document-updater with real HTTP servers -- unlike the docstore and filestore
// suites, which reach into the app they are testing.
const EXTERNAL = process.env.REALTIME_EXTERNAL === 'true'
const READY_TIMEOUT_MS = parseInt(
  process.env.REALTIME_EXTERNAL_TIMEOUT_MS || '30000',
  10
)

const { host, port } = Settings.internal.realTime

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
    `real-time on port ${port} was not ready within ${READY_TIMEOUT_MS}ms: ${lastError}`
  )
}

if (EXTERNAL) {
  await waitForExternalService()
} else {
  // Imported lazily so that a run against an external service never starts the
  // Node app or binds its port.
  await import('../app.js')
}

// Chai configuration
chai.should()
chai.use(chaiAsPromised)
chai.use(sinonChai)

// Global stubs
const sandbox = sinon.createSandbox()
const stubs = {
  logger: {
    debug: sandbox.stub(),
    log: sandbox.stub(),
    info: sandbox.stub(),
    warn: sandbox.stub(),
    err: sandbox.stub(),
    error: sandbox.stub(),
  },
}

// Mocha hooks
export const mochaHooks = {
  beforeEach() {
    this.logger = stubs.logger
  },

  afterEach() {
    sandbox.reset()
  },
}
