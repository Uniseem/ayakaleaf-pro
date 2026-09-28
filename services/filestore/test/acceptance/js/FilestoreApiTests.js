import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import { expect } from 'chai'

// A black-box contract suite for the filestore HTTP API.
//
// Everything here goes over HTTP and asserts only on status codes, headers and
// bodies, so the same suite holds any implementation of the service to the
// same contract. Run it with FILESTORE_EXTERNAL=true to point it at the Go
// port in services-go.
//
// It exists because FilestoreTests.js cannot serve that purpose: it reads TLS
// certificates from /certs at import time, needs fake-gcs-server, and calls
// FileHandler._TESTONLYSwapPersistorManager to replace the persistor inside
// the running service. All three require the service to share a process and an
// environment with the tests.

const HOST = '127.0.0.1'
const PORT = 3009
const BASE_URL = `http://${HOST}:${PORT}`
const EXTERNAL = process.env.FILESTORE_EXTERNAL === 'true'
const READY_TIMEOUT_MS = parseInt(
  process.env.FILESTORE_EXTERNAL_TIMEOUT_MS || '30000',
  10
)

const templateStore = process.env.TEMPLATE_FILES_BUCKET_NAME
const globalBlobStore = process.env.OVERLEAF_EDITOR_BLOBS_BUCKET
const projectBlobStore = process.env.OVERLEAF_EDITOR_PROJECT_BLOBS_BUCKET

let serverPromise

async function waitForService() {
  const deadline = Date.now() + READY_TIMEOUT_MS
  let lastError
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${BASE_URL}/status`)
      if (response.ok) return
      lastError = new Error(`status returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(`filestore was not ready in ${READY_TIMEOUT_MS}ms: ${lastError}`)
}

async function ensureRunning() {
  if (!serverPromise) {
    if (EXTERNAL) {
      serverPromise = waitForService()
    } else {
      const { default: app } = await import('../../../app.js')
      serverPromise = new Promise((resolve, reject) => {
        app.listen(PORT, HOST, err => (err ? reject(err) : resolve()))
      })
    }
  }
  return serverPromise
}

/** Reverses a padded id the way object-persistor's ProjectKey.format does. */
function projectKey(id) {
  const padded = String(id).padStart(9, '0')
  const reversed = padded.split('').reverse().join('')
  return path.join(reversed.slice(0, 3), reversed.slice(3, 6), reversed.slice(6))
}

/** Writes a blob straight into the filesystem store, as history would. */
function seedBlob(root, key, body) {
  const target = path.join(root, key)
  fs.mkdirSync(path.dirname(target), { recursive: true })
  fs.writeFileSync(target, body)
}

function objectId() {
  return crypto.randomBytes(12).toString('hex')
}

describe('filestore API', function () {
  before(async function () {
    if (!templateStore || !globalBlobStore || !projectBlobStore) {
      throw new Error(
        'set TEMPLATE_FILES_BUCKET_NAME, OVERLEAF_EDITOR_BLOBS_BUCKET and ' +
          'OVERLEAF_EDITOR_PROJECT_BLOBS_BUCKET to run this suite'
      )
    }
    await ensureRunning()
  })

  describe('status', function () {
    it('reports that it is up', async function () {
      const response = await fetch(`${BASE_URL}/status`)
      expect(response.status).to.equal(200)
    })
  })

  describe('template files', function () {
    let templateId, version, url
    const body = 'template contents\n'

    beforeEach(async function () {
      templateId = objectId()
      version = 42
      url = `${BASE_URL}/template/${templateId}/v/${version}/pdf`
      const response = await fetch(url, { method: 'POST', body })
      expect(response.status).to.equal(200)
    })

    it('stores and returns a file', async function () {
      const response = await fetch(url)
      expect(response.status).to.equal(200)
      expect(await response.text()).to.equal(body)
    })

    it('reports the size for a HEAD request', async function () {
      const response = await fetch(url, { method: 'HEAD' })
      expect(response.status).to.equal(200)
      expect(response.headers.get('content-length')).to.equal(
        String(Buffer.byteLength(body))
      )
    })

    it('serves a byte range', async function () {
      const response = await fetch(url, { headers: { Range: 'bytes=0-7' } })
      expect(response.status).to.be.oneOf([200, 206])
      expect(await response.text()).to.equal(body.slice(0, 8))
    })

    it('deletes a file', async function () {
      const response = await fetch(url, { method: 'DELETE' })
      expect(response.status).to.be.oneOf([200, 204])
      expect((await fetch(url)).status).to.equal(404)
    })

    it('returns 404 for a file that was never stored', async function () {
      const response = await fetch(
        `${BASE_URL}/template/${objectId()}/v/1/pdf`
      )
      expect(response.status).to.equal(404)
    })
  })

  describe('history blobs', function () {
    it('serves a global blob by hash', async function () {
      const hash = crypto.randomBytes(20).toString('hex')
      const body = 'global blob body'
      seedBlob(
        globalBlobStore,
        path.join(hash.slice(0, 2), hash.slice(2, 4), hash.slice(4)),
        body
      )
      const response = await fetch(`${BASE_URL}/history/global/hash/${hash}`)
      expect(response.status).to.equal(200)
      expect(await response.text()).to.equal(body)
    })

    it('serves a project blob by history id and hash', async function () {
      const hash = crypto.randomBytes(20).toString('hex')
      const historyId = '123456'
      const body = 'project blob body'
      seedBlob(
        projectBlobStore,
        path.join(projectKey(historyId), hash.slice(0, 2), hash.slice(2)),
        body
      )
      const response = await fetch(
        `${BASE_URL}/history/project/${historyId}/hash/${hash}`
      )
      expect(response.status).to.equal(200)
      expect(await response.text()).to.equal(body)
    })

    it('serves a byte range of a blob', async function () {
      const hash = crypto.randomBytes(20).toString('hex')
      const body = '0123456789'
      seedBlob(
        globalBlobStore,
        path.join(hash.slice(0, 2), hash.slice(2, 4), hash.slice(4)),
        body
      )
      const response = await fetch(`${BASE_URL}/history/global/hash/${hash}`, {
        headers: { Range: 'bytes=2-5' },
      })
      expect(response.status).to.be.oneOf([200, 206])
      expect(await response.text()).to.equal('2345')
    })

    it('returns 404 for a blob that does not exist', async function () {
      const hash = crypto.randomBytes(20).toString('hex')
      const response = await fetch(`${BASE_URL}/history/global/hash/${hash}`)
      expect(response.status).to.equal(404)
    })

    it('rejects a malformed hash rather than looking it up', async function () {
      for (const bad of ['nothex', '../../etc/passwd', 'ABCDEF']) {
        const response = await fetch(
          `${BASE_URL}/history/global/hash/${encodeURIComponent(bad)}`
        )
        expect(response.status, bad).to.equal(404)
      }
    })
  })

  describe('bucket access', function () {
    it('serves a file addressed by bucket and key', async function () {
      const templateId = objectId()
      const url = `${BASE_URL}/template/${templateId}/v/7/pdf`
      await fetch(url, { method: 'POST', body: 'bucket body' })

      const response = await fetch(
        `${BASE_URL}/bucket/${encodeURIComponent(templateStore)}/key/${templateId}/v/7/pdf`
      )
      expect(response.status).to.equal(200)
      expect(await response.text()).to.equal('bucket body')
    })
  })

  describe('routing', function () {
    it('returns 404 for an unknown route', async function () {
      expect((await fetch(`${BASE_URL}/no/such/route`)).status).to.equal(404)
    })
  })
})
