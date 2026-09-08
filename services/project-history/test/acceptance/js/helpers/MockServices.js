// A stand-in for nock, backed by real HTTP servers.
//
// The acceptance suite mocks history-v1, filestore and web with nock, which
// works by patching Node's http module inside the test process. That is fine
// while the service under test is required into that same process, and useless
// the moment it is a separate one: an external implementation makes real
// requests and reaches nothing.
//
// This exposes the same shape the tests already use -- nock(origin).get(path)
// .reply(code, body) and the modifiers around it -- and serves it from a real
// server listening on the origin's port. The tests do not change, and they can
// now judge any implementation, including the Node one started as its own
// process. That is what makes the comparison mean something.
//
// Set PROJECT_HISTORY_EXTERNAL=true to use it; without that the suite keeps
// using nock exactly as before.
//
// Two details of nock's shape are load-bearing here. reply() hands back the
// *scope*, not the interceptor, which is what lets a test chain a second
// expectation onto it and what it calls isDone() on. And persist, times,
// matchHeader and optionally are scope methods that reach the expectations
// around them, rather than methods on one expectation.
import fs from 'node:fs'
import http from 'node:http'
import { URL } from 'node:url'

/** Every scope that has been created, so cleanAll can reach them. */
const scopes = []
/** One server per origin, started on first use and left running. */
const servers = new Map()

/** One expectation: a method, a path, and what to answer. */
class Interceptor {
  constructor(scope, method, path, bodyMatcher) {
    this.scope = scope
    this.method = method
    this.path = path
    this.bodyMatcher = bodyMatcher
    this.queryMatcher = undefined
    this.header = scope.defaultHeader
    this.remaining = 1
    this.persistent = scope.defaultPersist
    this.isOptional = false
    this.done = false
    this.status = 200
    this.body = undefined
    this.filePath = undefined
    this.error = undefined
  }

  query(matcher) {
    this.queryMatcher = matcher
    return this
  }

  times(n) {
    this.remaining = n
    return this
  }

  persist() {
    this.persistent = true
    return this
  }

  optionally() {
    this.isOptional = true
    return this
  }

  matchHeader(name, value) {
    this.header = { name: name.toLowerCase(), value }
    return this
  }

  reply(status, body) {
    this.status = status
    this.body = body
    return this.scope.register(this)
  }

  /** replyWithFile answers with a fixture from disk. */
  replyWithFile(status, path) {
    this.status = status
    this.filePath = path
    return this.scope.register(this)
  }

  replyWithError(error) {
    this.error = error
    return this.scope.register(this)
  }

  spent() {
    return !this.persistent && this.remaining <= 0
  }

  matches(request) {
    if (this.method !== request.method) {
      return false
    }
    if (!this.pathMatches(request.pathname)) {
      return false
    }
    if (!this.queryMatches(request.query)) {
      return false
    }
    if (!this.headerMatches(request.headers)) {
      return false
    }
    return this.bodyMatches(request.body)
  }

  pathMatches(pathname) {
    if (this.path instanceof RegExp) {
      return this.path.test(pathname)
    }
    // A path given with a query string of its own has to match both parts.
    const [expectedPath] = String(this.path).split('?')
    return expectedPath === pathname
  }

  queryMatches(query) {
    const written = this.path instanceof RegExp ? '' : String(this.path)
    const [, expectedQuery] = written.split('?')

    if (this.queryMatcher === undefined) {
      // No matcher means the path was expected without a query, unless the
      // path carried one itself.
      if (expectedQuery) {
        return expectedQuery === query.toString()
      }
      return query.toString() === ''
    }
    if (this.queryMatcher === true) {
      return true
    }
    for (const [key, value] of Object.entries(this.queryMatcher)) {
      if (query.get(key) !== String(value)) {
        return false
      }
    }
    return true
  }

  headerMatches(headers) {
    if (!this.header) {
      return true
    }
    return headers[this.header.name] === this.header.value
  }

  bodyMatches(rawBody) {
    if (this.bodyMatcher === undefined) {
      return true
    }
    // A file is matched by its bytes. Reading it as text first would mangle
    // anything that is not valid UTF-8, which is most of what gets uploaded.
    if (Buffer.isBuffer(this.bodyMatcher)) {
      return Buffer.compare(this.bodyMatcher, rawBody) === 0
    }
    if (typeof this.bodyMatcher === 'function') {
      // The tests assert inside these, so a throw is a failed assertion and
      // has to reach the test rather than be swallowed as a non-match.
      return Boolean(this.bodyMatcher(parseBody(rawBody.toString('utf8'))))
    }
    if (typeof this.bodyMatcher === 'string') {
      return this.bodyMatcher === rawBody.toString('utf8')
    }
    return deepEqual(this.bodyMatcher, parseBody(rawBody.toString('utf8')))
  }

  consume() {
    this.done = true
    if (!this.persistent) {
      this.remaining -= 1
    }
  }
}

/** A Scope is everything registered against one origin. */
class Scope {
  constructor(origin) {
    this.origin = origin
    this.interceptors = []
    this.defaultPersist = false
    this.defaultHeader = undefined
    scopes.push(this)
    ensureServer(origin)
  }

  register(interceptor) {
    this.interceptors.push(interceptor)
    return this
  }

  get(path, bodyMatcher) {
    return new Interceptor(this, 'GET', path, bodyMatcher)
  }

  post(path, bodyMatcher) {
    return new Interceptor(this, 'POST', path, bodyMatcher)
  }

  put(path, bodyMatcher) {
    return new Interceptor(this, 'PUT', path, bodyMatcher)
  }

  delete(path, bodyMatcher) {
    return new Interceptor(this, 'DELETE', path, bodyMatcher)
  }

  head(path, bodyMatcher) {
    return new Interceptor(this, 'HEAD', path, bodyMatcher)
  }

  /** persist makes every expectation on this scope reusable. */
  persist() {
    this.defaultPersist = true
    for (const interceptor of this.interceptors) {
      interceptor.persistent = true
    }
    return this
  }

  /** times applies to the expectation just registered. */
  times(n) {
    const last = this.interceptors[this.interceptors.length - 1]
    if (last) {
      last.times(n)
    }
    return this
  }

  optionally() {
    const last = this.interceptors[this.interceptors.length - 1]
    if (last) {
      last.optionally()
    }
    return this
  }

  /** matchHeader is inherited by the expectations registered after it. */
  matchHeader(name, value) {
    this.defaultHeader = { name: name.toLowerCase(), value }
    const last = this.interceptors[this.interceptors.length - 1]
    if (last) {
      last.matchHeader(name, value)
    }
    return this
  }

  /** isDone reports whether every expectation on this scope was met. */
  isDone() {
    return this.interceptors.every(
      interceptor => interceptor.done || interceptor.isOptional
    )
  }

  clear() {
    this.interceptors = []
  }
}

/** The expectations registered for an origin, oldest scope first. */
function interceptorsFor(origin) {
  return scopes
    .filter(scope => scope.origin === origin)
    .flatMap(scope => scope.interceptors)
}

function ensureServer(origin) {
  if (servers.has(origin)) {
    return
  }
  const url = new URL(origin)
  // An origin written without a port means the default one for the scheme.
  // The retry callback is given as http://127.0.0.1/ping, and reading its port
  // as empty puts the server on an arbitrary one where nothing finds it.
  const port = url.port ? Number(url.port) : url.protocol === 'https:' ? 443 : 80

  const server = http.createServer((req, res) => {
    const chunks = []
    req.on('data', chunk => chunks.push(chunk))
    req.on('end', () => handle(origin, req, res, Buffer.concat(chunks)))
  })

  // A port that cannot be bound is reported once and left alone. The retry
  // callback is given on port 80, which is taken on most machines that run
  // anything else; the tests that need it will say so themselves rather than
  // the whole run dying on an unhandled listen error.
  server.on('error', error => {
    process.stderr.write(`  [mock] cannot serve ${origin}: ${error.message}
`)
  })
  server.listen(port, url.hostname)
  server.unref()
  servers.set(origin, server)
}

function handle(origin, req, res, rawBody) {
  const requested = new URL(req.url, origin)
  const request = {
    method: req.method,
    pathname: requested.pathname,
    query: requested.searchParams,
    headers: req.headers,
    body: rawBody,
  }

  let matched
  let failure
  for (const interceptor of interceptorsFor(origin)) {
    if (interceptor.spent()) {
      continue
    }
    try {
      if (interceptor.matches(request)) {
        matched = interceptor
        break
      }
    } catch (error) {
      // A body matcher that threw is a failed assertion inside the test.
      // Answering with the message keeps it visible rather than turning into
      // a confusing "no expectation matched".
      failure = error
      matched = interceptor
      break
    }
  }

  if (failure) {
    if (process.env.MOCK_SERVICES_VERBOSE) {
      // What the service actually sent. A failed body assertion says what was
      // expected and that something else arrived; without this, finding out
      // what means adding a print to the test.
      process.stderr.write(
        `  [mock] body did not match for ${request.method} ${request.pathname}:
` +
          `    ${bodyText(request.body)}
`
      )
    }
    matched.consume()
    res.writeHead(500, { 'Content-Type': 'text/plain' })
    res.end(`mock assertion failed: ${failure.message}`)
    return
  }
  if (!matched) {
    const query = request.query.toString()
    const what = `${request.method} ${request.pathname}${query ? '?' + query : ''}`
    // Worth saying out loud: an expectation that does not match is the usual
    // reason a test fails here, and a bare 404 in a log the service does not
    // print says nothing about which one.
    if (process.env.MOCK_SERVICES_VERBOSE) {
      process.stderr.write(`  [mock] no expectation for ${origin} ${what}
`)
    }
    // The request fails rather than being answered, because that is what nock
    // does with a call it was not told to expect. The difference is not
    // cosmetic: answering 404 tells the service the thing is not there, which
    // is a perfectly good answer that it will act on. A retry after a failed
    // blob check reached exactly that and went on to upload the blob, which is
    // the opposite of what the test was checking.
    res.destroy()
    return
  }

  matched.consume()
  if (process.env.MOCK_SERVICES_VERBOSE) {
    process.stderr.write(
      `  [mock] ${request.method} ${request.pathname} -> ${matched.status}
`
    )
  }

  if (matched.error) {
    // nock's replyWithError makes the request fail rather than answer.
    // Destroying the socket is the closest a real server can get.
    res.destroy()
    return
  }
  if (req.method === 'HEAD') {
    // A HEAD reply carries no body, whatever was registered.
    res.writeHead(matched.status)
    res.end()
    return
  }

  writeReply(res, matched)
}

function writeReply(res, interceptor) {
  if (interceptor.filePath) {
    res.writeHead(interceptor.status, { 'Content-Type': 'application/json' })
    res.end(fs.readFileSync(interceptor.filePath))
    return
  }

  const body = interceptor.body
  if (body === undefined || body === null) {
    res.writeHead(interceptor.status)
    res.end()
    return
  }
  if (Buffer.isBuffer(body)) {
    res.writeHead(interceptor.status, {
      'Content-Type': 'application/octet-stream',
    })
    res.end(body)
    return
  }
  if (typeof body === 'string') {
    res.writeHead(interceptor.status, { 'Content-Type': 'text/plain' })
    res.end(body)
    return
  }
  res.writeHead(interceptor.status, { 'Content-Type': 'application/json' })
  res.end(JSON.stringify(body))
}

function parseBody(raw) {
  if (raw === '') {
    return ''
  }
  try {
    return JSON.parse(raw)
  } catch {
    return raw
  }
}

/**
 * deepEqual compares two decoded bodies.
 *
 * Not by stringifying them: the service writes its JSON in whatever order it
 * builds the object, and the expectation in the test is written in the order
 * that reads best. Those are the same body and only a structural comparison
 * says so.
 */
function deepEqual(a, b) {
  if (a === b) {
    return true
  }
  if (a === null || b === null || typeof a !== 'object' || typeof b !== 'object') {
    return false
  }
  if (Array.isArray(a) !== Array.isArray(b)) {
    return false
  }
  if (Array.isArray(a)) {
    return a.length === b.length && a.every((item, i) => deepEqual(item, b[i]))
  }

  const keysA = Object.keys(a)
  const keysB = Object.keys(b)
  if (keysA.length !== keysB.length) {
    return false
  }
  return keysA.every(
    key => Object.prototype.hasOwnProperty.call(b, key) && deepEqual(a[key], b[key])
  )
}

/** nock is the facade the tests already use. */
function nock(origin) {
  return new Scope(origin)
}

nock.cleanAll = function cleanAll() {
  for (const scope of scopes) {
    scope.clear()
  }
  scopes.length = 0
}

// The servers keep running between tests: binding and unbinding a port for
// every test is slower and racier than leaving them up with nothing registered.
nock.close = function close() {
  for (const server of servers.values()) {
    server.close()
  }
  servers.clear()
}

export default nock

// bodyText renders a request body for a log line, as text when it is text and
// as a length when it is not.
function bodyText(body) {
  if (body == null) {
    return '(no body)'
  }
  if (Buffer.isBuffer(body)) {
    const text = body.toString('utf8')
    // eslint-disable-next-line no-control-regex
    return /[ -]/.test(text) ? `(${body.length} bytes)` : text
  }
  return typeof body === 'string' ? body : JSON.stringify(body)
}
