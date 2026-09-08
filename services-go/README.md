# services-go

Go ports of the smaller Overleaf services, built to be swapped in one at a time
without changing anything else in the stack.

| Service | Go binary | Port | Replaces | Node LOC |
| --- | --- | --- | --- | ---: |
| chat | `cmd/chat` | 3010 | `services/chat` | 996 |
| notifications | `cmd/notifications` | 3042 | `services/notifications` | 539 |
| linked-url-proxy | `cmd/linked-url-proxy` | 3066 | `services/linked-url-proxy` | 244 |
| docstore | `cmd/docstore` | 3016 | `services/docstore` | 1,403 |
| filestore | `cmd/filestore` | 3009 | `services/filestore` | 861 |
| real-time | `cmd/real-time` | 3026 | `services/real-time` | 3,193 |
| document-updater | `cmd/document-updater` | 3003 | `services/document-updater` | 5,834 |
| project-history | `cmd/project-history` | 3054 | `services/project-history` | 9,982 |

## Why these three

They are the leaves of the dependency graph: each one is reached only over
HTTP, owns its own Mongo collections, and has no shared mutable state with
another service. That makes them replaceable in isolation. They are also small
enough that the port can be read against the original line by line.

The point of starting here is not the memory they save — it is small — but to
get the replacement mechanism itself working end to end: build, conformance
test, cut over, roll back.

## What makes the swap safe

**Nothing else has to change.** `web` addresses every internal service through
an environment variable (`services/web/config/settings.defaults.js`):

```
CHAT_HOST  NOTIFICATIONS_HOST  LINKED_URL_PROXY_HOST
```

So a service is swapped by starting a different process on the same port. No
caller is recompiled, reconfigured, or even aware.

**The old tests are the contract.** The Node services' own acceptance suites
drive the service over HTTP. They now accept an environment variable that skips
starting the Node app and points them at whatever is already listening, which
is what `scripts/conformance.sh` drives in CI:

```bash
./scripts/conformance.sh chat            # runs services/chat's suite against cmd/chat
./scripts/conformance.sh notifications   # ditto for notifications
./scripts/conformance.sh all             # one after another, shared database
./scripts/conformance.sh --parallel      # all at once, isolated databases
```

It needs a reachable MongoDB — set `MONGO_HOST` or `MONGO_CONNECTION_STRING`.
`--parallel` gives each service its own database so their migrations cannot
race on the shared migrations collection; it finishes both suites in about
three seconds.

A port is only finished when the suite it inherited passes unchanged. As of
the last run both do, with the same test counts as the Node implementations:

| Suite | Node | Go |
| --- | ---: | ---: |
| services/chat acceptance | 29 passing | 29 passing |
| services/notifications acceptance | 18 passing | 18 passing |
| services/docstore acceptance (black-box files) | 39 passing | 39 passing |
| services/filestore contract suite | 13 passing | 13 passing |
| services/real-time acceptance | 484 passing, 1 failing | 484 passing, 1 failing |
| services/document-updater acceptance | 165 passing | 165 passing |
| services/project-history acceptance | 119 passing | 119 passing |

## Conformance is not enough: run it for real

The acceptance suites check the HTTP contract. They do not check that a service
can be *deployed*, and three defects got through them and were only found by
building the image and running the product:

1. **docstore had no `DOCSTORE_IMPL` toggle.** The port was complete and
   passing, and there was no way to select it. Nothing in a conformance run
   ever reads the runit scripts.
2. **The services did not read `OVERLEAF_MONGO_URL`.** A server-ce deployment
   configures Mongo through `OVERLEAF_CONFIG` → `/etc/overleaf/settings.js` →
   `OVERLEAF_MONGO_URL`, and sets neither `MONGO_CONNECTION_STRING` nor
   `MONGO_HOST`. The conformance script exports `MONGO_HOST` itself, so that
   path was never exercised: every service started, failed to reach
   127.0.0.1:27017, and was restarted by runit forever.
3. **`rev` and `version` were written as int64.** The Node driver stores a
   JavaScript number as int32, so documents carried a visible trace of which
   implementation wrote them. Harmless to queries, but it broke the promise
   that both can share one database.

So a port is finished when it passes the inherited suite **and** the product
builds, boots and works with it. What was checked on the live deployment:

| Check | Result |
| --- | --- |
| All four binaries present in the image | yes |
| runit selects Go when `*_IMPL=go` | all four |
| Runs alongside the Node services (real-time, project-history, clsi) | yes |
| Register, log in, create a project | yes |
| Compile LaTeX to PDF | success |
| Project chat through the Go service | send and read back |
| Go reads and updates a project Node wrote | rev 1 → 2 → 3, type preserved |
| Lines beginning with `$` survive storage | `$1.00`, `$foo` stored literally |
| Rollback: drop `*_IMPL`, restart | back on Node in 12s |

Two notes on building the image, both of which cost time to work out:

- `sharelatex/sharelatex-base` and `sharelatex/sharelatex` are **real public
  repositories on Docker Hub**. A local build is silently replaced by the
  published upstream image, which is old enough that its corepack has no
  `install` subcommand. Build the base under a name that cannot collide and
  pass it explicitly:

  ```bash
  docker build -f server-ce/Dockerfile-base -t ayakaleaf-base:local .
  docker build -f server-ce/Dockerfile \
    --build-arg OVERLEAF_BASE_TAG=ayakaleaf-base:local -t ayakaleaf-pro:go .
  ```

- The same collision means a `docker run sharelatex/sharelatex:latest` after a
  failed build quietly tests the *upstream* image. Verify what you are looking
  at before concluding anything from it.

### filestore: its own suite cannot run at all, and DELETE was broken

`FilestoreTests.js` never loads outside its docker-compose environment — its
`TestConfig.js` reads TLS certificates from `/certs/public.crt` at import time.
It also needs fake-gcs-server, and `FilestoreApp.runServer()` calls
`FileHandler._TESTONLYSwapPersistorManager()` to replace the persistor inside
the running service. It parametrises over eight backend shards, five of which
(`gcs`, `s3SSEC`, and three fallback/migration combinations) are unreachable
from server-ce's `settings.js`.

Writing a black-box replacement, `FilestoreApiTests.js`, immediately found that
**every DELETE returned 500**:

```
FileHandler.deleteFile is not a function
```

`deleteFile` was exported only under `promises`, while `FileController` calls
the callback-style top-level entry. The one-line fix is in this change; without
a suite that could run, nothing had exercised the path.

The Go port covers the `fs` and `s3` backends and refuses to start on any
other, rather than appearing to work against storage it cannot reach. GCS,
per-project client-side encryption and cross-backend migration are not ported.

### docstore: three acceptance files cannot judge an external service

`ArchiveDocsTests.js`, `GettingDocsFromArchiveTest.js` and
`DeletingDocsTests.js` reach into the running service and change its settings
mid-run:

```js
// services/docstore/test/acceptance/js/ArchiveDocsTests.js:210
Settings.docstore.keepSoftDeletedDocsArchived = true
```

That only takes effect when the service shares a process with the tests. Run
against a service in its own process they fail, and not because of the
implementation behind the port -- the **Node** service scores exactly the same
as the Go one when started externally:

| How the suite is run | Result |
| --- | --- |
| Node in-process (the default) | 97 passing |
| Node as its own process | 75 passing, 22 failing |
| Go as its own process | 75 passing, 22 failing |

So the conformance step runs the four files that are genuinely black-box, and
CI runs the Node baseline over the same four for comparison. Making the other
three usable would mean rewriting them to configure the service over its API or
environment instead of by assignment, which is a change to the Node test suite
rather than to the port.

### real-time: the suite needed no adaptation, and it found six defects

`services/real-time` is the opposite case. Its acceptance suite was always
black-box -- it connects over a real socket with the forked socket.io client,
seeds sessions straight into Redis, and mocks web and document-updater with
real HTTP servers -- so all 485 tests run against the Go binary unchanged.

One test cannot pass against any external service, for the same reason as
docstore's three: it asserts on the test process's own logger stub.

```js
// services/real-time/test/acceptance/js/LeaveDocTests.js:168
sinon.assert.calledWith(logger.debug, sinon.match.any,
  'ignoring request from client to leave room it is not in')
```

Run externally, Node fails exactly that test and nothing else, and so does Go:

| How the suite is run | Result |
| --- | --- |
| Node in-process (the default) | 485 passing |
| Node as its own process | 484 passing, 1 failing |
| Go as its own process | 484 passing, 1 failing |

Getting there took six fixes that the Go unit tests had not caught, four of
them in the socket.io layer:

1. **Rooms were cleared before the disconnect handler ran**, so the handler saw
   an empty room list and never unsubscribed the Redis channels behind them.
   Every project leaked an `editor-events:<id>` subscription for the life of
   the process.
2. **Closing a connection did not send a disconnect packet.** socket.io only
   suppresses its automatic reconnect when it is told the disconnect was
   deliberate; without the packet every boot -- a revoked project, a rejected
   session -- became a reconnect loop, and a booted client went on to rejoin
   the project it had just been removed from.
3. **The websocket read limit was 32KB**, the default of the websocket library.
   The service is supposed to answer an oversized update with an error the
   client understands, and it can only do that if the update reaches it.
4. **Frames queued just before a close were dropped**, which is exactly the
   frame that says why the connection is closing.
5. `userRemovedFromProject` carries one user id per event argument, not a list
   inside the first one.
6. A malformed rpc has to be answered. A client waiting on a callback that
   never comes waits forever.

### real-time: four more defects that only a browser could find

The suite above passes at parity, and the service still did not work. Opening
the editor against it found four defects in a row, none of which any test could
have reached:

1. **The session cookie never verified.** express writes it with
   `encodeURIComponent`, so a browser sends `s%3A<id>.<sig>`; `cookie-parser`
   decodes it first, and Go's `Request.Cookie` does not. Every connection was
   rejected as an invalid session. Both acceptance suites set the `Cookie`
   header themselves, unencoded, so neither could see it.
2. **The server never sent heartbeats.** The direction is easy to get
   backwards: in socket.io 0.9 the *server* sends one every 25 seconds and the
   client answers. A client that hears nothing for its timeout closes the
   connection itself, so every session dropped about 30 seconds in. The suite's
   longest wait is 500ms.
3. **The Redis key schema was the wrong one.** server-ce replaces it wholesale
   in `/etc/overleaf/settings.js`, dropping the hash-tag braces:
   `PendingUpdates:<id>` where the services' own defaults say
   `PendingUpdates:{<id>}`. Edits were queued under a name document-updater was
   not reading. Nothing failed and nothing was logged -- the editor showed the
   typing, then declared the document out of sync. Both suites assert the
   upstream names, so both agreed with the wrong answer.
4. **One editor event has a payload that is not an argument list.**
   document-updater's canary probe publishes a bare object, which Node accepts
   because it has no types to disagree with. A strict envelope rejected it --
   and that decode is the one every editor event goes through.

The first three are configuration and protocol: things a service only meets
once it is wired into a real deployment with a real client. That is now three
services in a row where the conformance suite passed and the deployment did
not, which is the argument for doing both.

### document-updater: the suite did not say what the deployment needed

document-updater passed its inherited suite with 165 tests and none failing,
and then could not create a project. web asks it to record the new document,
and it answered 501.

The port had been treating `historyRangesSupport` as a mode it could decline.
It is not a mode. Every project created by this version of web has
`overleaf.history.rangesSupportEnabled` set, so it is the ordinary path, and
declining it means the service cannot be deployed at all. The suite never said
so: only two of its tests turn the flag on, and both of those assert on a spy
inside the service, so they cannot judge an external one either way.

What the flag asks for is that a document be recorded twice over. The editor
shows the text with tracked deletions taken out; the history keeps them in. So
every position the editor works in is short by the length of the tracked
deletions before it, and everything sent to the history carries a second
position measured in the longer text -- and a second length, where a comment
spans a deletion. It is what makes a restored version come back with its
comments attached to the same words. That is now ported and compared against
the real `RangesManager` and `HistoryConversions` over 300,000 random documents
each.

The lesson is the same as the three above, one step further on: an inherited
suite says what the service must not get wrong, not what a deployment will ask
of it. `scripts/verify-live-editing.sh` is the answer to that -- it drives the
live stack through the path a person editing a document goes through, and it is
run against the Node service first so that its 22 checks mean something.

### What document-updater does not port

**history-ot.** A second OT type, with its own storage format, its own
operation shape and its own conversion to history. A document using it is
refused with a 422 rather than read as though its content were lines. server-ce
does not enable it; the acceptance tests for it are the one category
`scripts/conformance.sh` skips.

**The diff library.** `diffAsShareJsOp` turns a whole-document write into an
edit, and the answer is not unique: several correct diffs rebuild the same
text, and which one is chosen is what the other editors are shown and what goes
into the history. The Go port of diff-match-patch on offer works in characters
and in bytes where the original works in UTF-16 code units, scores one side of
a boundary with the wrong pattern, and leaves emptied components in its result.
So `internal/textdiff` is a port of the algorithm itself, compared against the
real `DiffCodec` over a million random document pairs.

### real-time is stateful, so the switch is visible

The four services before it are stateless: swapping one is invisible because
nothing is holding a connection. real-time holds every open editor session, so
restarting it under a different implementation disconnects everyone currently
typing and they reconnect a moment later. No work is lost -- edits already live
in document-updater -- but unlike the others, this cut-over is something users
see.

**Data formats are untouched.** Same collections, same field names, same BSON
types — including the detail that `Date.now()` is stored as a BSON *double*,
because that is what the Node driver writes for a JS number. Both
implementations can therefore run against one database at the same time, which
is what makes shadow-traffic comparison possible.

## Build and test

Builds and tests run in GitHub Actions: [`.github/workflows/go_services.yml`](../.github/workflows/go_services.yml).
That is the authority, because the acceptance and conformance suites need
MongoDB and cannot run on a machine without Docker. Three jobs:

| Job | What it proves |
| --- | --- |
| `check` | gofmt, `go vet`, unit tests under `-race` |
| `build` | cross-compiles for linux/amd64 and linux/arm64, uploads the binaries |
| `conformance` | runs each acceptance suite twice: Node first as a baseline, then the Go binary |

The double run is the point. If the Node baseline fails, the environment is
broken and the Go result says nothing; if the baseline passes and the Go run
fails, the port is at fault. There is no ambiguity to argue about.

Locally, as a smoke test only:

```bash
cd services-go
make build   # go build -o bin/ ./cmd/...
make test    # go test ./...
make lint    # gofmt check + go vet
```

Go 1.25 or newer is required (set by the module's dependencies).

## Cutting a service over

Each runit script in `server-ce/runit/` picks its implementation at startup:

| Service | Set this to use Go |
| --- | --- |
| chat | `CHAT_IMPL=go` |
| notifications | `NOTIFICATIONS_IMPL=go` |
| linked-url-proxy | `LINKED_URL_PROXY_IMPL=go` |
| docstore | `DOCSTORE_IMPL=go` |
| filestore | `FILESTORE_IMPL=go` |
| real-time | `REALTIME_IMPL=go` |

Any other value, including unset, runs the Node service exactly as before. Both
implementations ship in the image, so **rollback is one environment variable
and a service restart** — no rebuild, no redeploy.

Suggested sequence per service:

1. `./scripts/conformance.sh <service>` — inherited suite passes.
2. Run both in staging and compare responses on mirrored traffic.
3. Flip `*_IMPL=go` in production, watch `timer_http_request` and the error
   rate for a week.
4. Only then consider deleting the Node implementation.

## Compatibility details worth knowing

These are the places where matching the original took deliberate effort:

- **ObjectId validation** (`internal/oid`). The Node driver's
  `ObjectId.isValid()` accepts a 24-character hex string *or* any 12-character
  string, and the services rely on it to decide between 400 and 200. Accepting
  only hex would have changed behaviour, so both forms are accepted.
- **Metrics labels** (`internal/obsv`). `timer_http_request` carries the same
  `method`/`status_code`/`path` labels, with `path` derived from the route
  template the way `@overleaf/metrics`' `getRoutePath()` derives it, plus the
  `app`/`host` default labels. An existing acceptance test scrapes `/metrics`
  and asserts on `path="project_{projectId}_messages"`; it passes.
- **Logs** (`internal/logx`). Records are bunyan-shaped
  (`name`/`hostname`/`pid`/`level`/`msg`/`time`/`v`) with the same numeric
  levels, so existing log tooling and `LOG_LEVEL` keep working.
- **JSON field order** (`internal/bsonjson`). Raw Mongo documents are rendered
  with fields in stored order and ObjectIds as hex strings, matching
  `res.json()` on a document from the Node driver. Go maps would have sorted
  the keys.
- **Validation order** in chat. The Node service validates the request body
  (exegesis) before path parameters (`readContext`) before the controller's own
  checks. The order decides *which* 400 message a bad request gets, and the
  acceptance suite asserts on those messages, so it is preserved.

## Deliberate differences from the Node services

Each of these is a case where copying the original exactly would have meant
copying a defect. They are listed so the choice is visible rather than silent.

1. **Empty `insertMany` batches.** `cloneThreads` and `duplicateRoomToOtherRoom`
   call `insertMany([])` when there is nothing to copy, which the Mongo driver
   rejects with "Batch cannot be empty" — so cloning a project with no comment
   threads fails with a 500. The Go port treats it as a no-op.
2. **Health-check cleanup.** `HealthCheckController.cleanupNotifications` passes
   a string where an ObjectId is required, so it never deletes anything and
   smoke-test documents accumulate. The Go port passes the ObjectId.
3. **Invalid `blockedNetworks` entries.** The Node proxy discovers an
   unparseable CIDR at request time and returns 500 for every proxied request.
   The Go port fails at startup, where an operator will actually see it.
4. **Invalid `?limit=` on chat message listing.** `parseInt('abc')` yields NaN,
   which the Node service hands to the driver. The Go port falls back to the
   default of 50.
5. **Upstream error bodies in the proxy.** A non-2xx upstream response produces
   the same status code but a differently worded `Error: ...` body.
6. **project-history's health check cannot pass in server-ce.** `HealthChecker`
   builds an `ObjectId` from `Settings.history.healthCheck.project_id`, which
   server-ce never sets, so `new ObjectId('')` throws and `/health_check`
   answers 500 on every deployment. The Go port checks the lock and answers
   200 when no project has been named to check against, and does the full
   flush-and-read when one has.

## Known gaps, carried over unchanged

- The proxy enforces `MAX_UPLOAD_SIZE` from the upstream `Content-Length`
  header only. An upstream that omits or understates it can still stream more
  than the limit. This matches the Node service; fixing it would risk
  truncating legitimate downloads and belongs in its own change.

## Layout

```
cmd/                       one main package per service
internal/
  bsonjson/                order-preserving BSON -> JSON, matching res.json()
  chat/                    chat store, formatter, HTTP handlers
  config/                  the same env vars the Node settings files read
  httpx/                   JSON helpers and graceful shutdown
  logx/                    bunyan-compatible slog handler
  mongox/                  Mongo connection
  notifications/           notifications store, handlers, health check
  obsv/                    Prometheus metrics compatible with @overleaf/metrics
  oid/                     ObjectId parsing with Node's exact semantics
  proxy/                   SSRF address policy and the proxying handler
  histmodel/               the history's data model: changes, snapshots, the
                           scan-based text operation, tracked changes, comments
  projecthistory/          the queue, the resync, the translation between the
                           editor's operations and the history's, and the read
                           path the editor's history view uses
  textdiff/                diff-match-patch's diff half, in UTF-16 code units
  textot/                  the ShareJS text type the editor speaks
scripts/conformance.sh     runs the Node acceptance suites against these binaries
```

## What is deliberately not here

`web`, `clsi` and `history-v1` are untouched. `web` alone is roughly 350k lines
and holds every Pro feature; `clsi` has its bottleneck outside Node, waiting on
TeX Live; and `history-v1` is the store the history is actually kept in, which
is the last thing to move rather than the next.

One thing inside `project-history` is deliberately partial: a project whose
editor speaks the history's own operation type sends operations already in the
history's form. Those are passed through and composed by the history's own
rules, which is what the inherited suite exercises, but the port does not
implement that operation type end to end -- `document-updater` does not either,
and neither does server-ce use it.
