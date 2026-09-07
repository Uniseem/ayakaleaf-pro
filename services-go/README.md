# services-go

Go ports of the smaller Overleaf services, built to be swapped in one at a time
without changing anything else in the stack.

| Service | Go binary | Port | Replaces | Node LOC |
| --- | --- | --- | --- | ---: |
| chat | `cmd/chat` | 3010 | `services/chat` | 996 |
| notifications | `cmd/notifications` | 3042 | `services/notifications` | 539 |
| linked-url-proxy | `cmd/linked-url-proxy` | 3066 | `services/linked-url-proxy` | 244 |
| docstore | `cmd/docstore` | 3016 | `services/docstore` | 1,403 |

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
| Runs alongside the Node services (filestore, real-time, project-history) | yes |
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
scripts/conformance.sh     runs the Node acceptance suites against these binaries
```

## What is deliberately not here

`web`, `clsi`, `document-updater`, `real-time`, `history-v1`, `project-history`
and `filestore` are untouched. `web` alone is roughly 350k lines and holds every
Pro feature; the rest either have their bottleneck outside Node (`clsi` waits on
TeX Live) or carry subtle state that a rewrite should not take on until this
mechanism has proven itself on something small.
