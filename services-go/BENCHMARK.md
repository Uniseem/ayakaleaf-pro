# What the port actually bought

The reason for doing this was that the deployment was memory-heavy and slow.
That is a claim about numbers, so here are the numbers.

Both runs are the same container, the same database, the same project, on the
same machine, minutes apart. The only thing that differs between them is which
implementation the runit scripts started — every service is selected by a
`*_IMPL` environment variable, so the switch is `docker compose up -d` and
nothing else moves. `clsi`, `history-v1` and `web` are Node in both runs and
serve as the control: if the numbers moved for them too, the measurement would
be measuring the machine rather than the change.

Measured with `scripts/benchmark.sh` and `scripts/throughput.sh`, from inside
the container, on an 8-core Xeon with 11 GB of memory.

## Memory

Resident set size, which is what the machine has to find. Megabytes.

| Service | Node | Go | |
| --- | ---: | ---: | ---: |
| chat | 110.8 | 19.8 | −82% |
| docstore | 99.3 | 37.2 | −63% |
| document-updater | 98.3 | 19.9 | −80% |
| filestore | 82.5 | 11.0 | −87% |
| linked-url-proxy | 85.4 | 11.2 | −87% |
| notifications | 117.0 | 15.2 | −87% |
| project-history | 108.5 | 22.2 | −80% |
| real-time | 100.7 | 15.9 | −84% |
| **the eight ported** | **802.5** | **152.4** | **−81%** |
| | | | |
| clsi *(control)* | 94.6 | 93.3 | −1% |
| history-v1 *(control)* | 113.7 | 116.7 | +3% |
| web *(control)* | 374.7 | 380.7 | +2% |
| **whole container** | **1385.6** | **743.2** | **−46%** |

The three controls move by a couple of percent, which is the noise floor. The
eight that were replaced fall by a factor of five.

Worth saying plainly: a Node service that does almost nothing still holds
about 100 MB, and eight of them held 800 MB between them. That is not what any
of them needed; it is what the runtime costs to have running.

## Time, one request at a time

Milliseconds, median of 40 requests after 5 warmups.

| Operation | Node | Go | |
| --- | ---: | ---: | ---: |
| docupdater.get-doc | 3.1 | 1.2 | 2.6× |
| docupdater.set-doc | 3.8 | 1.3 | 2.9× |
| docupdater.get-project-docs | 3.3 | 1.4 | 2.4× |
| docupdater.flush | 2.9 | 1.2 | 2.4× |
| docstore.get-doc | 3.2 | 1.6 | 2.0× |
| docstore.get-all-docs | 3.2 | 1.7 | 1.9× |
| chat.list | 5.4 | 2.6 | 2.1× |
| chat.send | 6.0 | 3.4 | 1.8× |
| notifications.list | 1.1 | 0.5 | 2.2× |
| filestore.upload | 1.2 | 0.5 | 2.4× |
| filestore.download | 1.1 | 0.5 | 2.2× |
| history.flush | 4.0 | 2.0 | 2.0× |
| history.version | 13.0 | 9.5 | 1.4× |
| history.updates | 12.7 | 9.7 | 1.3× |
| history.diff | 15.6 | 11.9 | 1.3× |
| web.project-page *(control)* | 5.5 | 5.0 | 1.1× |

Two things to read out of this rather than the ratio alone.

The history endpoints gain least, and that is the right answer rather than a
disappointing one: most of those nine to sixteen milliseconds is history-v1,
which is still Node, and Mongo. The part that was replaced was never the
expensive part of them.

And the absolute numbers are small. Saving two milliseconds on a keystroke is
invisible to one person typing. It is not invisible to a server with two
hundred of them.

## Time, with people editing at once

Sixteen concurrent clients against one endpoint for ten seconds, requests
completed.

| Endpoint | Node | Go | |
| --- | ---: | ---: | ---: |
| docupdater.get-doc | 262 req/s | 489 req/s | 1.9× |
| docstore.get-doc | 652 req/s | 786 req/s | 1.2× |
| history.version | 205 req/s | 315 req/s | 1.5× |
| chat.list | 353 req/s | 745 req/s | 2.1× |

`docstore.get-doc` gains least because it is a Mongo query with a little
wrapping: the database is doing the work in both runs and it is the same
database.

## web, which was not ported

The biggest single consumer left, and the one most obviously worth asking
about. Measured with `scripts/web-timing.sh`.

It runs as two processes:

| | Resident |
| --- | ---: |
| `web` app.mjs | 187 MB |
| `web` app.mjs | 205 MB |
| **total** | **392 MB** |

And the pages it serves:

| | Status | p50 | p95 | Size |
| --- | --- | ---: | ---: | ---: |
| `GET /login` | 200 | 4.9 ms | 6.5 ms | 13 kB |
| `GET /project` (the project list) | 200 | 21.5 ms | 31.8 ms | 30 kB |
| `GET /socket.io/socket.io.js` | 200 | 1.3 ms | 2.9 ms | 44 kB |
| `GET /project/:id` (the editor) | 429 | 4.4 ms | 7.7 ms | — |

The project list is the heaviest thing measured here and it is web doing real
work: the session, the projects query, the tags, and rendering the page with
its bootstrap payload. Twenty-one milliseconds.

The editor page could not be measured: hitting it twenty times in a row is
what its rate limiter exists to stop, and what came back was the 429. That is
the right behaviour and it left the number unmeasured.

**web's problem is not that it is slow. It is that it is 392 MB.** And 392 MB
of Node heap is the one thing this whole exercise is not allowed to touch, so
the only measurable complaint about the largest remaining service has no
remedy that is in scope.

## What this does not say

- One machine, one project, warm caches, no other load. A busy server behaves
  differently and this does not predict how.
- Nothing here measures a LaTeX compile, which is where a person actually
  waits. That is `clsi` and TeX Live, and neither was touched.
- The biggest single consumer left is `web` at 380 MB, and it is out of scope:
  it holds every feature and serves the frontend.
- Throughput was measured at one concurrency level. The shape of the curve
  under real load was not measured.

## So: is it faster?

Yes, and by a factor worth having — about twice the speed and a fifth of the
memory on everything that was replaced — but the honest framing is that the
memory is the big result and the latency is the small one. Six hundred and
forty megabytes came back on an eleven gigabyte machine, which is the
difference between the deployment having room and not. The two milliseconds
per request are real and they compound under load, but nobody typing will
notice them.
